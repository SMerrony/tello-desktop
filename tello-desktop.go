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
	"fmt"
	"os/exec"
	"sync"
	"time"

	"gobot.io/x/gobot"
	"gobot.io/x/gobot/platforms/dji/tello"
	"gobot.io/x/gobot/platforms/joystick"
)

const telloUDPport = "8890"

// control mapping for T-Flight flight controller
const (
	takeOffCtrl    = joystick.SquarePress
	landCtrl       = joystick.XPress
	stopCtrl       = joystick.CirclePress
	moveLRCtrl     = joystick.RightX
	moveFwdBkCtrl  = joystick.RightY
	moveUpDownCtrl = joystick.LeftY
	turnLRCtrl     = joystick.LeftX
)

var (
	goLeft, goRight, goFwd, goBack,
	goUp, goDown, clockwise, antiClockwise int
	moveMutex sync.RWMutex
)

func main() {
	joystickAdaptor := joystick.NewAdaptor()
	stick := joystick.NewDriver(joystickAdaptor, "tflightHotasX")

	drone := tello.NewDriver(telloUDPport)

	work := func() {

		// start external mplayer instance...
		// the -vo X11 parm allows it to run nicely inside a virtual machine
		// setting the FPS to 60 seems to produce smoother video
		mplayer := exec.Command("mplayer", "-vo", "x11", "-fps", "60", "-")
		mplayerIn, _ := mplayer.StdinPipe()
		if err := mplayer.Start(); err != nil {
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
			if _, err := mplayerIn.Write(pkt); err != nil {
				fmt.Println(err)
			}
		})

		// display some events on console
		drone.On(tello.TakeoffEvent, func(data interface{}) { fmt.Println("Taken Off") })
		drone.On(tello.LandingEvent, func(data interface{}) { fmt.Println("Landing") })
		//drone.On(tello.LightStrengthEvent, func(data interface{}) { fmt.Println("Light Strength Event") })
		drone.On(tello.FlightDataEvent, func(data interface{}) {
			fmt.Println("Flight Data")
			fd := data.(*tello.FlightData)
			fmt.Printf("Batt: %d%%, Height: %.1fm, Hover: %t, Sky: %t, Ground: %t, Open: %t\n",
				fd.BatteryPercentage,
				float32(fd.Height)/10,
				fd.DroneHover,
				fd.EmSky, fd.EmGround, fd.EmOpen)
		})

		// joystick button presses
		stick.On(takeOffCtrl, func(data interface{}) { drone.TakeOff() })

		stick.On(landCtrl, func(data interface{}) { drone.Land() })

		stick.On(stopCtrl, func(data interface{}) {
			drone.Left(0)
			drone.Right(0)
			drone.Up(0)
			drone.Down(0)
		})

		// joystick stick movements
		// move left/right
		stick.On(moveLRCtrl, func(data interface{}) {
			js16 := int(data.(int16))
			moveMutex.Lock()
			defer moveMutex.Unlock()
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
			moveMutex.Lock()
			defer moveMutex.Unlock()
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
			moveMutex.Lock()
			defer moveMutex.Unlock()
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
			moveMutex.Lock()
			defer moveMutex.Unlock()
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
	}

	robot := gobot.NewRobot("tello",
		[]gobot.Connection{joystickAdaptor},
		[]gobot.Device{drone, stick},
		work,
	)

	robot.Start()
}
