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

// known controllers
const (
	keyboardCtl      = "keyboard"
	dualshock4Ctl    = "dualshock4"
	tflightHotasXCtl = "tflightHotasX"
)

// keyboard control mapping
const (
	takeOffKey   = sdl.K_t
	landKey      = sdl.K_l
	palmlandKey  = sdl.K_p
	panicKey     = sdl.K_SPACE
	moveLeftKey  = sdl.K_LEFT
	moveRightKey = sdl.K_RIGHT
	moveFwdKey   = sdl.K_UP
	moveBkKey    = sdl.K_DOWN
	turnLeftKey  = sdl.K_a
	turnRightKey = sdl.K_d
	moveUpKey    = sdl.K_w
	moveDownKey  = sdl.K_s
	bounceKey    = sdl.K_b
	quitKey      = sdl.K_q
	helpKey      = sdl.K_h
)

const keyMoveIncr = 15

// joystick control mapping
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
	winUpdatePeriod                         = 333 * time.Millisecond
	fontPath                                = "../../assets/Inconsolata-Bold.ttf"
	bigFontSize, medFontSize, smallFontSize = 32, 24, 12
)

// program flags
var (
	controlFlag = flag.String("control", "keyboard", "Gobot controller <keyboard|dualshock4|tflightHotasX")
	joyHelpFlag = flag.Bool("joyhelp", false, "Print help for joystick control mapping and exit")
	keyHelpFlag = flag.Bool("keyhelp", false, "Print help for keyboard control mapping and exit")
)

var (
	robot       *gobot.Robot
	useKeyboard bool // if this is set we use keyboard input, otherwise joystick
	keyChan     chan sdl.Keysym
	goLeft, goRight, goFwd, goBack,
	goUp, goDown, clockwise, antiClockwise int
	moveMu       sync.RWMutex
	flightData   *tello.FlightData
	flightMsg    = "Idle"
	flightDataMu sync.RWMutex
	wifiData     *tello.WifiData
	wifiDataMu   sync.RWMutex

	// These are just for development purposes
	dummyFD = new(tello.FlightData)
	dummyWD = new(tello.WifiData)
)

var (
	bigFont, medFont, smallFont *ttf.Font
	window                      *sdl.Window
	surface                     *sdl.Surface
	textColour                  = sdl.Color{R: 255, G: 128, B: 64, A: 255}
)

func printKeyHelp() {
	fmt.Print(
		`Tello Desktop Keyboard Control Mapping

<Cursor Keys> Move Left/Right/Forward/Backward
W|A|S|D       W: Up, S: Down, A: Turn Left, D: Turn Right
<SPACE>       Hover (stop all movement)
T             Takeoff
L             Land
P             Palm Land
B             Bounce (on/off)
Q             Quit
H             Print Help
`)
}

func printJoystickHelp() {
	fmt.Print(
		`Tello Desktop Joystick Control Mapping

Right Stick  Forward/Backward/Left/Right
Left Stick   Up/Down/Turn
Triangle     Takeoff
X            Land
Circle       Hover (stop all movement)
L1           Bounce (on/off)
L2           Palm Land
`)
}

func main() {
	flag.Parse()
	if *keyHelpFlag {
		printKeyHelp()
		os.Exit(0)
	}
	if *joyHelpFlag {
		printJoystickHelp()
		os.Exit(0)
	}

	// catch termination signal
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		exitNicely()
	}()

	// FIXME Just for development...
	flightData = dummyFD
	wifiData = dummyWD

	switch *controlFlag {
	case keyboardCtl:
		fmt.Println("Setting up Keyboard as controller")
		useKeyboard = true
		keyChan = make(chan sdl.Keysym, 3)
	case dualshock4Ctl:
		fmt.Println("Setting up DualShock4 controller")
		useKeyboard = false
	case tflightHotasXCtl:
		fmt.Println("Setting up T-Flight HOTAS-X controller")
		useKeyboard = false
	default:
		log.Fatalf("Unknown joystick type %s", *controlFlag)
	}

	setupWindow()

	joystickAdaptor := joystick.NewAdaptor()
	stick := joystick.NewDriver(joystickAdaptor, *controlFlag)

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
			drone.Forward(0)
			drone.Backward(0)
			drone.Up(0)
			drone.Down(0)
			drone.Clockwise(0)
			drone.CounterClockwise(0)
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

		gobot.Every(winUpdatePeriod, func() { updateWindow() })

		if useKeyboard {
			for key := range keyChan {
				switch key.Sym {
				case takeOffKey:
					drone.TakeOff()
				case landKey:
					drone.Land()
				case palmlandKey:
					drone.PalmLand()
				case panicKey:
					drone.Left(0)
					drone.Right(0)
					drone.Forward(0)
					drone.Backward(0)
					drone.Up(0)
					drone.Down(0)
					drone.Clockwise(0)
					drone.CounterClockwise(0)
				case bounceKey:
					drone.Bounce()
				case moveLeftKey:
					drone.Left(keyMoveIncr)
				case moveRightKey:
					drone.Right(keyMoveIncr)
				case moveFwdKey:
					drone.Forward(keyMoveIncr)
				case moveBkKey:
					drone.Backward(keyMoveIncr)
				case moveUpKey:
					drone.Up(keyMoveIncr)
				case moveDownKey:
					drone.Down(keyMoveIncr)
				case turnLeftKey:
					drone.CounterClockwise(keyMoveIncr * 2)
				case turnRightKey:
					drone.Clockwise(keyMoveIncr * 2)
				case quitKey, sdl.K_ESCAPE:
					exitNicely()
				case helpKey:
					printKeyHelp()
				}
			}
		}
	}

	if useKeyboard {
		robot = gobot.NewRobot("tello",
			[]gobot.Connection{},
			[]gobot.Device{drone},
			work,
		)
	} else {
		robot = gobot.NewRobot("tello",
			[]gobot.Connection{joystickAdaptor},
			[]gobot.Device{drone, stick},
			work,
		)
	}

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

	go sdlEventListener()
}

func updateWindow() {
	surface.FillRect(nil, 0)

	renderTextAt("Steve's Tello Desktop", bigFont, 155, 5)
	renderTextAt(time.Now().Format(time.RFC1123), medFont, 150, 50)
	flightDataMu.RLock()
	if flightData == nil {
		renderTextAt("No flight data available", bigFont, 100, 200)
		flightDataMu.RUnlock()
	} else {
		ht := fmt.Sprintf("Height: %.1fm", float32(flightData.Height)/10)
		gs := fmt.Sprintf("Ground Speed:  %d m/s", flightData.GroundSpeed)
		fs := fmt.Sprintf("Speeds - Fwd: %d m/s", flightData.NorthSpeed)
		ls := fmt.Sprintf("Side: %d m/s", flightData.EastSpeed)
		ds := math.Sqrt(float64(flightData.NorthSpeed*flightData.NorthSpeed) + float64(flightData.EastSpeed*flightData.EastSpeed))
		dstr := fmt.Sprintf("Derived: %.1f m/s", ds)
		loc := fmt.Sprintf("Hover: %c, Open: %c, Sky: %c, Ground: %c",
			boolToYN(flightData.DroneHover),
			boolToYN(flightData.EmOpen),
			boolToYN(flightData.EmSky),
			boolToYN(flightData.EmGround))
		bp := fmt.Sprintf("Battery: %d%%", flightData.BatteryPercentage)
		ftr := fmt.Sprintf("Remaining Flight Time: %ds", flightData.DroneFlyTimeLeft)
		//ws := fmt.Sprintf("WiFi - Strength: %d Interference: %d", flightData.WifiStrength, flightData.WifiDisturb)
		msg := flightMsg

		flightDataMu.RUnlock()

		// render the text outside of the data lock for best concurrency
		renderTextAt(ht, bigFont, 220, 100)
		renderTextAt(gs, medFont, 200, 140)
		renderTextAt(fs, medFont, 20, 180)
		renderTextAt(ls, medFont, 290, 180)
		renderTextAt(dstr, medFont, 460, 180)
		renderTextAt(loc, medFont, 20, 240)
		//renderTextAt(ws, medFont, 20, 460)
		renderTextAt(bp, medFont, 20, 500)
		renderTextAt(ftr, medFont, 300, 500)
		if msg != "" {
			renderTextAt(flightMsg, medFont, 20, 550)
		}
	}

	wifiDataMu.RLock()
	ws := fmt.Sprintf("WiFi - Strength: %d Interference: %d", wifiData.Strength, wifiData.Disturb)
	wifiDataMu.RUnlock()

	renderTextAt(ws, medFont, 20, 460)

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

func sdlEventListener() {
	var event sdl.Event
	for {
		event = sdl.WaitEvent()
		switch event.(type) {
		case *sdl.QuitEvent: // catch window closure
			fmt.Println("Window Quit event")
			exitNicely()

		case *sdl.KeyboardEvent:
			fmt.Println("Keyboard Event")
			// only send key presses for now
			if event.(*sdl.KeyboardEvent).Type == sdl.KEYDOWN {
				keyChan <- event.(*sdl.KeyboardEvent).Keysym
			}
		}
	}
}
