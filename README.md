# tello-desktop
A functioning desktop testbed for flying the DJI Tello drone via [Gobot](https://github.com/hybridgroup/gobot).

_Play with this entirely at your own risk - it's not the author's fault if you lose your drone
or damage it, or anything else, when using this software._

This is an unpolished, but working, utility to try out features of the dji/tello platform in the Gobot library.

It currently provides 
* live video via mplayer (must be installed separately)
* basic control from the keyboard
* joystick control via a Dualshock 4 or Thrustmaster T-Flight flight controller
* flight status window

Only tested on GNU/Linux - it almost certainly won't work as-is on other platforms.

N.B. This app may use an in-development version of the Gobot package.
Any released versions should build with a contemporary release of Gobot.

## Build
For the moment you will need to clone gobot from github and checkout the DEV branch.
Then, in the cmd/gobot directory build the binary with this command...
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

N.B. For keyboard control to work the Tello Desktop window must have focus.

Once you have landed the drone, stop the program with Ctrl-C, some errors will appear - this is normal.
