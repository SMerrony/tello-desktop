# tello-desktop
A functioning desktop testbed for flying the Ryze Tello drone via various APIs.

Currently there are versions:
* One using [Gobot](https://github.com/hybridgroup/gobot) in cmd/tello-gobot and 
* One using [tello](https://github.com/SMerrony/tello) in cmd/tello-package.

_Play with this entirely at your own risk - it's not the author's fault if you lose your drone
or damage it, or anything else, when using this software._

Both versions currently provide... 
* live video via mplayer (must be installed separately)
* control from the keyboard
* control via a Dualshock 4 game controller
* flight status window

The Gobot version also supports the Thrustmaster T-Flight flight controller.

The tello-package version also supports picture taking, flips and a few more flight commands.

Only tested on GNU/Linux - it almost certainly won't work as-is on other platforms.

The Gobot version requires at least version 1.11.0 of the Gobot package.
Any released versions should build with a contemporary release of Gobot.

## Build
In either the cmd/gobot or cmd/package directory build the binary with this command...
``go build -o tello-desktop``
Before attempting to run the app you must have mplayer installed.

## Usage
* Centre the throttle control at the mid-position if using a flight controller
* Turn on the Tello
* Wait for it to initialise (flashing orange LED)
* Connect your computer to the Tello WiFi
* Run tello-desktop from a terminal window

After a couple of seconds a video feed should appear - if it doesn't, then something is wrong so do not attempt to fly the Tello!

Use the `-joyhelp` option to see the joystick control mappings.

Use the `-keyhelp` option to see the keyboard control mappings.  Be aware that in keyboard mode Tello motion continues until you
counteract it, or stop the Tello with the space bar.

If you find that mplayer takes over the whole screen (rather than being in its own window), then try the -x11 option which may help.

N.B. To control the Tello the Tello Desktop window must have focus.

Once you have landed the drone, stop the program with the Q key.
