import sdnotify
import signal
import uinput

with uinput.Device([uinput.KEY_SPACE], name="workshop-test-input"):
    sdnotify.SystemdNotifier().notify("READY=1")
    signal.pause()
