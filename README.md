# tello-desktop
A functioning desktop testbed for flying the DJI Tello drone via [Gobot](https://github.com/hybridgroup/gobot).

_Play with this entirely at your own risk - it's not the author's fault if you lose your drone
or damage it, or anything else, when using this software._

This is an unpolished, but working, utility to try out features of the dji/tello platform in the Gobot library.

It currently provides 
* live video via mplayer (must be installed separately)
* joystick control via a Thrustmaster T-Flight flight controller
* stream of drone info to the console

Only tested on GNU/Linux - it almost certainly won't work as-is on other platforms.

N.B. This app may use an in-development version of the Gobot package.
Any released versions should build with a contemporary release of Gobot.

## Usage
* Centre the throttle control at the mid-position
* Turn on the Tello
* Wait for it to initialise (flashing orange LED)
* Connect your computer to the Tello WiFi
* Run tello-desktop from a terminal window

After a couple of seconds a video feed should appear - if it doesn't, then something is wrong so do not attempt to fly the Tello!

If all is OK then you can launch the Tello from the flight controller.  In the default configuration the following controls are available...
* Square - Take Off
* Cross (X) - Land
* Circle - Panic - Stop all movement
* Throttle - Move Up/Down - centre is steady, forward to go up, back to do down
* Left Twist - Rotate drone left/right
* Right Joystick - Conventional movement control, twist not currently used

Once you have landed the drone, stop the program with Ctrl-C, some errors will appear - this is normal.
