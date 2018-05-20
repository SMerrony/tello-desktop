// tello-desktop.go

// Copyright (C) 2018  Steve Merrony

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"

	"gobot.io/x/gobot"
	"gobot.io/x/gobot/platforms/dji/tello"
	"gobot.io/x/gobot/platforms/joystick"
)

const telloUDPport = "8890"

// known joysticks
const (
	dualshock4    = "dualshock4"
	tflightHotasX = "tflightHotasX"
)

// control mapping
const (
	takeOffCtrl    = joystick.TrianglePress
	landCtrl       = joystick.XPress
	stopCtrl       = joystick.CirclePress
	moveLRCtrl     = joystick.RightX
	moveFwdBkCtrl  = joystick.RightY
	moveUpDownCtrl = joystick.LeftY
	turnLRCtrl     = joystick.LeftX
	bounceCtrl     = joystick.L1Press
	palmLandCtrl   = joystick.L2Press
)

const (
	winTitle                                = "Tello Desktop"
	winWidth, winHeight                     = 800, 600
	fontPath                                = "./assets/Inconsolata-Bold.ttf"
	bigFontSize, medFontSize, smallFontSize = 32, 24, 12
)

// program flags
var (
	joystickFlag = flag.String("joystick", tflightHotasX, "Gobot joystick ID <dualshock4|tflightHotasX>")
)

var (
	robot *gobot.Robot
	goLeft, goRight, goFwd, goBack,
	goUp, goDown, clockwise, antiClockwise int
	moveMu       sync.RWMutex
	flightData   *tello.FlightData
	flightMsg    = "Idle"
	flightDataMu sync.RWMutex
	wifiData     *tello.WifiData
	wifiDataMu   sync.RWMutex
	dummyFD      = new(tello.FlightData) // FIXME Just for debugging
)

var (
	bigFont, medFont, smallFont *ttf.Font
	window                      *sdl.Window
	surface                     *sdl.Surface
	textColour                  = sdl.Color{R: 255, G: 128, B: 64, A: 255}
)

func main() {

	// catch termination signal
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		exitNicely()
	}()

	// FIXME Just for debugging...
	flightData = dummyFD

	flag.Parse()
	switch *joystickFlag {
	case dualshock4:
		fmt.Println("Setting up DualShock4 controller")
	case tflightHotasX:
		fmt.Println("Setting up T-Flight HOTAS-X controller")
	default:
		log.Fatalf("Unknown joystick type %s", *joystickFlag)
	}

	setupWindow()

	//kbd := keyboard.NewDriver()
	joystickAdaptor := joystick.NewAdaptor()
	stick := joystick.NewDriver(joystickAdaptor, *joystickFlag)

	drone := tello.NewDriver(telloUDPport)

	work := func() {

		// start external mplayer instance...
		// the -vo X11 parm allows it to run nicely inside a virtual machine
		// setting the FPS to 60 seems to produce smoother video
		player := exec.Command("mplayer", "-nosound", "-vo", "x11", "-fps", "60", "-")
		//player := exec.Command("ffplay", "-framedrop", "-an", "-i", "pipe:0")
		playerIn, _ := player.StdinPipe()
		if err := player.Start(); err != nil {
			fmt.Println(err)
			return
		}

		// start video feed when drone connects
		drone.On(tello.ConnectedEvent, func(data interface{}) {
			fmt.Println("Connected")
			drone.StartVideo()
			drone.SetVideoEncoderRate(2)
			gobot.Every(500*time.Millisecond, func() {
				drone.StartVideo()
			})
		})

		// send each video frame recieved to mplayer
		drone.On(tello.VideoFrameEvent, func(data interface{}) {
			pkt := data.([]byte)
			if _, err := playerIn.Write(pkt); err != nil {
				fmt.Println(err)
			}
		})

		// display some events on console
		drone.On(tello.TakeoffEvent, func(data interface{}) {
			flightDataMu.Lock()
			flightMsg = "Taking Off"
			flightDataMu.Unlock()
		})
		drone.On(tello.LandingEvent, func(data interface{}) {
			flightDataMu.Lock()
			flightMsg = "Landing"
			flightDataMu.Unlock()
		})
		//drone.On(tello.LightStrengthEvent, func(data interface{}) { fmt.Println("Light Strength Event") })
		drone.On(tello.FlightDataEvent, func(data interface{}) {
			flightDataMu.Lock()
			flightData = data.(*tello.FlightData)
			if flightData.BatteryLow {
				flightMsg = "Battery Low"
			}
			if flightData.BatteryLower {
				flightMsg = "Battery Lower"
			}
			flightDataMu.Unlock()

		})

		drone.On(tello.WifiDataEvent, func(data interface{}) {
			wifiDataMu.Lock()
			wifiData = data.(*tello.WifiData)
			wifiDataMu.Unlock()
		})

		// joystick button presses
		stick.On(takeOffCtrl, func(data interface{}) {
			drone.TakeOff()
			fmt.Println("Taking off")
		})

		stick.On(landCtrl, func(data interface{}) {
			drone.Land()
			fmt.Println("Landing")
		})

		stick.On(stopCtrl, func(data interface{}) {
			fmt.Println("Stopping (Hover)")
			drone.Left(0)
			drone.Right(0)
			drone.Up(0)
			drone.Down(0)
		})

		stick.On(bounceCtrl, func(data interface{}) {
			fmt.Println("Bounce start/stop")
			drone.Bounce()
		})

		stick.On(palmLandCtrl, func(data interface{}) {
			fmt.Println("Palm Landing")
			drone.PalmLand()
		})

		// joystick stick movements
		// move left/right
		stick.On(moveLRCtrl, func(data interface{}) {
			js16 := int(data.(int16))
			moveMu.Lock()
			defer moveMu.Unlock()
			switch {
			case js16 < 0:
				goLeft = js16 / -328
				drone.Left(goLeft)
				fmt.Printf("GoLeft set to %d from raw data %d\n", goLeft, js16)
			case js16 > 0:
				goRight = js16 / 328
				drone.Right(goRight)
				fmt.Printf("GoRight set to %d from raw data %d\n", goRight, js16)
			default:
				goLeft, goRight = 0, 0
				drone.Left(0)
				drone.Right(0)
				fmt.Println("GoLeft & GoRight set to 0")
			}
		})
		// move forward/backward
		stick.On(moveFwdBkCtrl, func(data interface{}) {
			js16 := int(data.(int16))
			moveMu.Lock()
			defer moveMu.Unlock()
			switch {
			case js16 > 0:
				goBack = js16 / 328
				drone.Backward(goBack)
				fmt.Printf("GoBack set to %d from raw data %d\n", goBack, js16)
			case js16 < 0:
				goFwd = js16 / -328
				drone.Forward(goFwd)
				fmt.Printf("GoFwd set to %d from raw data %d\n", goFwd, js16)
			default:
				goBack, goFwd = 0, 0
				drone.Backward(0)
				drone.Forward(0)
				fmt.Println("GoBack & GoFwdset to 0")
			}
		})
		// move up/down
		stick.On(moveUpDownCtrl, func(data interface{}) {
			js16 := int(data.(int16))
			moveMu.Lock()
			defer moveMu.Unlock()
			switch {
			case js16 > 0:
				goDown = js16 / 328
				drone.Down(goDown)
				fmt.Printf("GoDown set to %d from raw data %d\n", goDown, js16)
			case js16 < 0:
				goUp = js16 / -328
				drone.Up(goUp)
				fmt.Printf("GoUp set to %d from raw data %d\n", goUp, js16)
			default:
				goDown, goUp = 0, 0
				drone.Down(0)
				drone.Up(0)
				fmt.Println("GoBack & GoFwd set to 0")
			}
		})
		// turn left/right
		stick.On(turnLRCtrl, func(data interface{}) {
			js16 := int(data.(int16))
			moveMu.Lock()
			defer moveMu.Unlock()
			switch {
			case js16 < 0:
				antiClockwise = js16 / -328
				drone.CounterClockwise(antiClockwise)
				fmt.Printf("antiClockwise set to %d from raw data %d\n", antiClockwise, js16)
			case js16 > 0:
				clockwise = js16 / 328
				drone.Clockwise(clockwise)
				fmt.Printf("clockwise set to %d from raw data %d\n", clockwise, js16)
			default:
				antiClockwise, clockwise = 0, 0
				drone.CounterClockwise(0)
				drone.Clockwise(0)
				fmt.Println("clockwise & antiClockwise set to 0")
			}
		})

		// // keyboard commands
		// kbd.On(keyboard.Key, func(data interface{}) {
		// 	key := data.(keyboard.KeyEvent)
		// 	switch key.Key {
		// 	case keyboard.Q, keyboard.Escape:
		// 		exitNicely()
		// 	}
		// })

		gobot.Every(time.Second, func() { updateWindow() })
	}

	robot = gobot.NewRobot("tello",
		[]gobot.Connection{joystickAdaptor},
		[]gobot.Device{drone, stick},
		work,
	)

	robot.Start()
}

func setupWindow() {
	var err error

	if err = sdl.Init(sdl.INIT_EVERYTHING); err != nil {
		panic(err)
	}
	if err = ttf.Init(); err != nil {
		panic(err)
	}
	bigFont, err = ttf.OpenFont(fontPath, bigFontSize)
	if err != nil {
		log.Fatalf("Failed to open font %s due to %v", fontPath, err)
	}
	medFont, _ = ttf.OpenFont(fontPath, medFontSize)
	smallFont, _ = ttf.OpenFont(fontPath, smallFontSize)
	window, err = sdl.CreateWindow(winTitle, sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED, winWidth, winHeight, sdl.WINDOW_SHOWN)
	if err != nil {
		panic(err)
	}
	surface, err = window.GetSurface()
	if err != nil {
		panic(err)
	}
	surface.FillRect(nil, 0)
	renderTextAt("Hello, Tello!", bigFont, 200, 200)
	window.UpdateSurface()
}

func updateWindow() {
	surface.FillRect(nil, 0)

	renderTextAt("Steve's Tello Desktop", bigFont, 155, 5)
	renderTextAt(time.Now().Format(time.RFC1123), medFont, 150, 50)
	flightDataMu.RLock()
	if flightData == nil {
		renderTextAt("No flight data available", bigFont, 100, 200)
	} else {
		ht := fmt.Sprintf("Height: %.1fm", float32(flightData.Height)/10)
		renderTextAt(ht, medFont, 20, 100)

		gs := fmt.Sprintf("Ground Speed:  %d m/s", flightData.GroundSpeed)
		renderTextAt(gs, medFont, 20, 140)
		ns := fmt.Sprintf("North Speed:   %d m/s", flightData.NorthSpeed)
		renderTextAt(ns, medFont, 20, 160)
		es := fmt.Sprintf("East Speed:    %d m/s", flightData.EastSpeed)
		renderTextAt(es, medFont, 20, 180)
		ds := math.Sqrt(float64(flightData.NorthSpeed*flightData.NorthSpeed) + float64(flightData.EastSpeed*flightData.EastSpeed))
		dstr := fmt.Sprintf("Derived Speed: %.1f m/s", ds)
		renderTextAt(dstr, medFont, 20, 200)

		loc := fmt.Sprintf("Hover: %c, Open: %c, Sky: %c, Ground: %c",
			boolToYN(flightData.DroneHover),
			boolToYN(flightData.EmOpen),
			boolToYN(flightData.EmSky),
			boolToYN(flightData.EmGround))
		renderTextAt(loc, medFont, 20, 240)

		bp := fmt.Sprintf("Battery: %d%%", flightData.BatteryPercentage)
		renderTextAt(bp, medFont, 20, 500)
		ftr := fmt.Sprintf("Remaining Flight Time: %ds", flightData.DroneFlyTimeLeft)
		renderTextAt(ftr, medFont, 300, 500)
		if flightMsg != "" {
			renderTextAt(flightMsg, medFont, 20, 550)
		}
	}
	flightDataMu.RUnlock()
	wifiDataMu.RLock()
	ws := fmt.Sprintf("WiFi Strength: %d,  Interference: %d", wifiData.Strength, wifiData.Disturb)
	wifiDataMu.RUnlock()
	renderTextAt(ws, medFont, 20, 480)
	window.UpdateSurface()
}

func renderTextAt(what string, font *ttf.Font, x int32, y int32) {
	render, err := font.RenderUTF8Solid(what, textColour)
	if err != nil {
		panic(err)
	}
	rect := &sdl.Rect{X: x, Y: y}
	err = render.Blit(nil, surface, rect)
	if err != nil {
		panic(err)
	}
}

func exitNicely() {

	robot.Stop()
	window.Destroy()
	bigFont.Close()
	medFont.Close()
	smallFont.Close()
	sdl.Quit()
	os.Exit(0)
}

func boolToYN(b bool) byte {
	if b {
		return 'Y'
	}
	return 'N'
}
