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

	"github.com/SMerrony/tello"
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

const keyMoveIncr = 5000

// joystick control mapping
const (
	//takeOffCtrl    = joystick.TrianglePress
	//landCtrl       = joystick.XPress
	//stopCtrl       = joystick.CirclePress
	moveLRCtrl     = 3 // joystick.RightX
	moveFwdBkCtrl  = 4 // joystick.RightY
	moveUpDownCtrl = 1 // joystick.LeftY
	turnLRCtrl     = 0 // joystick.LeftX
	//bounceCtrl     = joystick.L1Press
	//palmLandCtrl   = joystick.L2Press
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
	drone       tello.Tello
	useKeyboard bool // if this is set we use keyboard input, otherwise joystick
	keyChan     chan sdl.Keysym
	sticks      tello.StickMessage
	joy         *sdl.Joystick
	goLeft, goRight, goFwd, goBack,
	goUp, goDown, clockwise, antiClockwise int
	moveMu       sync.RWMutex
	flightData   tello.FlightData
	flightMsg    = "Idle"
	flightDataMu sync.RWMutex
	wifiDataMu   sync.RWMutex
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

	j := sdl.NumJoysticks()
	log.Printf("Number of Joysticks detected: %d\n", j)
	h, _ := sdl.NumHaptics()
	log.Printf("Number of haptics controllers %d\n", h)
	if j > 0 {
		joy = sdl.JoystickOpen(0)
		if joy == nil {
			log.Println("Error opening connection to joystick")
			j = 0
		}
		//stickChan, _ = drone.StartStickListener()
	}
	//joystickAdaptor := joystick.NewAdaptor()
	//stick := joystick.NewDriver(joystickAdaptor, *controlFlag)

	err := drone.ControlConnectDefault()
	if err != nil {
		log.Fatalf("Tello ControlConnectDefault() failed with error %v", err)
	}

	err = drone.VideoConnectDefault()
	if err != nil {
		log.Fatalf("Tello VideoConnectDefault() failed with error %v", err)
	}

	// start external mplayer instance...
	// the -vo X11 parm allows it to run nicely inside a virtual machine
	// setting the FPS to 60 seems to produce smoother video
	player := exec.Command("mplayer", "-nosound", "-fps", "60", "-")

	playerIn, err := player.StdinPipe()
	if err != nil {
		log.Fatalf("Unable to get STDIN for mplayer %v", err)
	}
	if err := player.Start(); err != nil {
		log.Fatalf("Unable to start mplayer - %v", err)
		return
	}

	_ = playerIn
	// start video feed when drone connects
	drone.StartVideo()
	go func() {
		for {
			drone.StartVideo()
			time.Sleep(500 * time.Millisecond)
		}
	}()

	//_ = playerIn
	go func() {
		for {
			vbuf := <-drone.VideoChan
			_, err := playerIn.Write(vbuf)
			if err != nil {
				log.Fatalf("Error writing to mplayer %v\n", err)
			}
			//log.Println("Wrote a v buf")
		}
	}()

	// subscribe to FlightData events and askfor updates every 50ms
	fdChan, _ := drone.StreamFlightData(false, 50)
	go func() {
		for {
			tmpFD := <-fdChan
			flightDataMu.Lock()
			flightData = tmpFD
			if flightData.BatteryLow {
				flightMsg = "Battery Low"
			}
			if flightData.BatteryLower {
				flightMsg = "Battery Lower"
			}
			flightDataMu.Unlock()
		}
	}()
	log.Println("Checkpoint 1")
	// // joystick button presses
	// stick.On(takeOffCtrl, func(data interface{}) {
	// 	drone.TakeOff()
	// 	fmt.Println("Taking off")
	// })

	// stick.On(landCtrl, func(data interface{}) {
	// 	drone.Land()
	// 	fmt.Println("Landing")
	// })

	// stick.On(stopCtrl, func(data interface{}) {
	// 	fmt.Println("Stopping (Hover)")
	// 	drone.Left(0)
	// 	drone.Right(0)
	// 	drone.Forward(0)
	// 	drone.Backward(0)
	// 	drone.Up(0)
	// 	drone.Down(0)
	// 	drone.Clockwise(0)
	// 	drone.CounterClockwise(0)
	// })

	// stick.On(bounceCtrl, func(data interface{}) {
	// 	fmt.Println("Bounce start/stop")
	// 	drone.Bounce()
	// })

	// stick.On(palmLandCtrl, func(data interface{}) {
	// 	fmt.Println("Palm Landing")
	// 	drone.PalmLand()
	// })

	go func() {
		for {
			updateWindow()
			time.Sleep(winUpdatePeriod)
		}
	}()
	log.Println("Checkpoint 1a")

	drone.SetVideoBitrate(tello.Vbr1M5)
	log.Println("Checkpoint 2")
	go sdlEventListener()
	log.Println("Checkpoint 3")
	if useKeyboard {
		for key := range keyChan {
			switch key.Sym {
			case takeOffKey:
				drone.TakeOff()
			case landKey:
				drone.Land()
			// case palmlandKey:
			// 	drone.PalmLand()
			case panicKey:
				drone.Hover()
				// case bounceKey:
				// 	drone.Bounce()
			case moveLeftKey:
				drone.Left(25)
			case moveRightKey:
				drone.Right(25)
			case moveFwdKey:
				drone.Forward(25)
			case moveBkKey:
				drone.Backward(25)
			case moveUpKey:
				drone.Up(50)
			case moveDownKey:
				drone.Down(50)
			case turnLeftKey:
				drone.TurnLeft(50)
			case turnRightKey:
				drone.TurnRight(50)
			case quitKey, sdl.K_ESCAPE:
				exitNicely()
			case helpKey:
				printKeyHelp()
			}
		}
	}
	log.Println("Checkpoint 4")
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

	//go sdlEventListener()
}

func updateWindow() {
	surface.FillRect(nil, 0)

	renderTextAt("Steve's Tello Desktop", bigFont, 155, 5)
	renderTextAt(time.Now().Format(time.RFC1123), medFont, 150, 50)
	flightDataMu.RLock()
	if !drone.ControlConnected() {
		renderTextAt("No flight data available", bigFont, 100, 200)
		flightDataMu.RUnlock()
	} else {
		ht := fmt.Sprintf("Height: %.1fm", float32(flightData.Height)/10)
		gs := fmt.Sprintf("Ground Speed:  %d m/s", flightData.GroundSpeed)
		fs := fmt.Sprintf("Speeds - Fwd: %d m/s", flightData.NorthSpeed)
		ls := fmt.Sprintf("Side: %d m/s", flightData.EastSpeed)
		ds := math.Sqrt(float64(flightData.NorthSpeed*flightData.NorthSpeed) + float64(flightData.EastSpeed*flightData.EastSpeed))
		dstr := fmt.Sprintf("Derived: %.1f m/s", ds)
		loc := fmt.Sprintf("Flying: %c, Hover: %c, Ground: %c, Windy: %c",
			boolToYN(flightData.Flying),
			boolToYN(flightData.DroneHover),
			boolToYN(flightData.OnGround),
			boolToYN(flightData.WindState))
		bp := fmt.Sprintf("Battery: %d%%  Over Temp: %c", flightData.BatteryPercentage, boolToYN(flightData.OverTemp))
		ftr := fmt.Sprintf("Remaining - Flight Time: %ds, Battery: %d", flightData.DroneFlyTimeLeft, flightData.DroneFlyTimeLeft)
		ws := fmt.Sprintf("WiFi - Strength: %d Interference: %d", flightData.WifiStrength, flightData.WifiInterference)
		msg := flightMsg

		flightDataMu.RUnlock()

		// render the text outside of the data lock for best concurrency
		renderTextAt(ht, bigFont, 220, 100)
		renderTextAt(gs, medFont, 200, 140)
		renderTextAt(fs, medFont, 20, 180)
		renderTextAt(ls, medFont, 290, 180)
		renderTextAt(dstr, medFont, 460, 180)
		renderTextAt(loc, medFont, 20, 240)
		renderTextAt(ws, medFont, 20, 360)
		renderTextAt(bp, medFont, 20, 400)
		renderTextAt(ftr, medFont, 20, 440)
		if msg != "" {
			renderTextAt(flightMsg, medFont, 20, 550)
		}
	}

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

	// window.Destroy()
	// bigFont.Close()
	// medFont.Close()
	// smallFont.Close()
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

		case *sdl.JoyAxisEvent:
			handleJoyAxisEvent(event.(*sdl.JoyAxisEvent))

		case *sdl.KeyboardEvent:
			fmt.Println("Keyboard Event")
			// only send key presses for now
			if event.(*sdl.KeyboardEvent).Type == sdl.KEYDOWN {
				keyChan <- event.(*sdl.KeyboardEvent).Keysym
			}
		}
	}
}

func handleJoyAxisEvent(ev *sdl.JoyAxisEvent) {
	switch ev.Axis {
	case turnLRCtrl: // lx
		sticks.Lx = ev.Value
	case moveUpDownCtrl: // ly
		sticks.Ly = -ev.Value
	case 2: // l2
	case moveLRCtrl: // rx
		sticks.Rx = ev.Value
	case moveFwdBkCtrl: //
		log.Printf("Got js RY value: %d\n", ev.Value)
		sticks.Ry = -ev.Value
	case 5: // r2
	}
	drone.UpdateSticks(sticks)
}
