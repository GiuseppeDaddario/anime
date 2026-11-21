"""Optional real-player smoke test against a paused local clip and mpv IPC socket.

Start the player on a disposable clip at least 30 seconds long, with a 2s-12s intro:
anime --player --pause --input-ipc-server=/tmp/mpv-preview.sock \
    --script-opts-append=animewatcher-intro_start=2 \
    --script-opts-append=animewatcher-intro_end=12 clip.mp4
python tests/check_mpv_overlay.py --socket /tmp/mpv-preview.sock --screenshots /tmp
The test clicks inside/outside the overlay and checks normal mpv keyboard input.
"""

import argparse
import json
from pathlib import Path
import socket
import time


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--socket", required=True)
    parser.add_argument("--screenshots", type=Path)
    args = parser.parse_args()
    connection = socket.socket(socket.AF_UNIX)
    connection.settimeout(5)
    connection.connect(args.socket)
    stream = connection.makefile("rb")
    request_id = 0

    def command(*values):
        nonlocal request_id
        request_id += 1
        connection.sendall((json.dumps({"command": values, "request_id": request_id}) + "\n").encode())
        while True:
            line = stream.readline()
            if not line:
                raise RuntimeError("mpv closed its IPC socket")
            response = json.loads(line)
            if response.get("request_id") == request_id:
                if response["error"] != "success":
                    raise RuntimeError(f"{values}: {response['error']}")
                return response.get("data")

    def get(name):
        return command("get_property", name)

    def wait_for(predicate, description):
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            value = predicate()
            if value:
                return value
            time.sleep(0.03)
        raise AssertionError(description)

    def screenshot(name):
        if args.screenshots:
            args.screenshots.mkdir(parents=True, exist_ok=True)
            command("screenshot-to-file", str(args.screenshots / name), "window")

    def button():
        return get("user-data/animewatcher/skip-button") or {}

    def glass():
        return [path for path in get("glsl-shaders") if path not in user_shaders]

    # This test intentionally changes playback position: use a disposable local clip.
    assert get("duration") >= 30, "Use a local test clip at least 30 seconds long"
    user_shaders = get("glsl-shaders")
    command("set_property", "pause", True)
    command("seek", 20, "absolute+exact")
    wait_for(lambda: abs(get("time-pos") - 20) < 0.08, "Initial exact seek failed")
    assert button().get("intro_end") == 12, "Start mpv with the 2s-12s intro from this file's docstring"
    wait_for(lambda: not button().get("visible"), "The button showed outside the intro")
    command("seek", 3, "absolute+exact")
    found = wait_for(lambda: button().get("visible") and button(), "The button did not appear in the intro")
    wait_for(lambda: len(glass()) == 1, "The glass shader was not added under the button")
    x, y = round(found["x"] + found["width"] / 2), round(found["y"] + found["height"] / 2)
    command("mouse", x, y)
    wait_for(lambda: button().get("hovered"), "Button did not receive mouse hover")
    screenshot("animewatcher-mpv-hover.png")
    fullscreen = get("fullscreen")
    command("mouse", x, y, 0, "double")
    assert get("fullscreen") == fullscreen, "Double-clicking the button triggered fullscreen"
    command("mouse", x, y, 0, "single")
    wait_for(lambda: abs(get("time-pos") - 12) < 0.08, "Click did not seek exactly to the intro's end")
    assert get("pause") is True, "Click changed the paused state"
    wait_for(lambda: not button().get("visible"), "Button did not disappear after clicking")
    wait_for(lambda: not glass(), "The glass shader stayed after the button left")
    screenshot("animewatcher-mpv-clicked.png")

    command("mouse", 30, 120, 0, "single")
    assert abs(get("time-pos") - 12) < 0.08, "Click outside the button triggered a seek"
    command("keypress", "SPACE")
    wait_for(lambda: get("pause") is False, "Normal mpv spacebar playback control was intercepted")
    command("keypress", "SPACE")
    wait_for(lambda: get("pause") is True, "Normal mpv pause control was intercepted")
    command("script-message-to", "skip_intro", "show")
    command("script-message-to", "skip_intro", "skip")
    assert not button().get("visible"), "The button reappeared after the intro"
    assert get("time-pos") < 13, "The intro was skipped twice"

    command("seek", 4, "absolute+exact")
    wait_for(lambda: button().get("visible"), "Seeking back into the intro did not offer the skip again")
    screenshot("animewatcher-mpv-default.png")
    print("PASS: the button showed only in the 2s-12s intro over its glass, a click sought exactly to 12s and "
          "dismissed it; outside clicks, paused state, double-click isolation, spacebar controls and re-arming passed.")
    connection.close()


if __name__ == "__main__":
    main()
