"""render.py records cligram watch in a pseudo terminal, as it shows the
run events.sh writes, and draws what the terminal showed into docs/demo.gif.

    cd docs/demo && go build -o cligram ../../cmd/cligram && python3 render.py

It needs pyte and Pillow (pip install pyte pillow), and the fonts that
harness/pixels/fonts.sh fetches.
"""

import fcntl, os, pty, select, struct, sys, termios, time

import pyte
from PIL import Image, ImageDraw, ImageFont

COLS, ROWS = 108, 23
FONT = "../../harness/pixels/fonts/JetBrainsMono-Regular.ttf"
SIZE = 15
SCRIPT = "printf '$ ./events.sh | cligram watch flow.mmd\\n'; sleep 0.6; ./events.sh | ./cligram watch flow.mmd"

# The terminal's colors: cligram paints lines bright black, labels blue,
# states green, cyan, red and yellow.
PALETTE = {
    "default": (216, 225, 227), "black": (15, 20, 22), "red": (255, 107, 107),
    "green": (79, 209, 139), "brown": (226, 176, 74), "yellow": (226, 176, 74),
    "blue": (110, 168, 254), "magenta": (199, 146, 234), "cyan": (60, 198, 193),
    "white": (216, 225, 227), "brightblack": (107, 122, 125), "brightwhite": (255, 255, 255),
}
BACKGROUND = (15, 20, 22)


def record():
    """Runs the demo in a pseudo terminal, and gives what it wrote, with when."""
    pid, fd = pty.fork()
    if pid == 0:
        os.environ["TERM"] = "xterm-256color"
        os.execv("/bin/sh", ["/bin/sh", "-c", SCRIPT])
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
    chunks, start = [], time.time()
    while True:
        ready, _, _ = select.select([fd], [], [], 0.05)
        if not ready:
            continue
        try:
            data = os.read(fd, 65536)
        except OSError:
            break
        if not data:
            break
        # Bubble Tea asks the terminal for its background and the cursor.
        if b"\x1b]11;?" in data:
            os.write(fd, b"\x1b]11;rgb:0f0f/1414/1616\x1b\\")
        if b"\x1b[6n" in data:
            os.write(fd, b"\x1b[1;1R")
        chunks.append((time.time() - start, data))
    os.waitpid(pid, 0)
    return chunks


def screens(chunks):
    """The screens the terminal showed, each with when it came, up to the
    run's end, when the full screen view leaves."""
    screen = pyte.Screen(COLS, ROWS)
    stream = pyte.ByteStream(screen)
    out = []
    for at, data in chunks:
        ended = b"\x1b[?1049l" in data
        if ended:
            data = data[: data.index(b"\x1b[?1049l")]
        # The full screen view starts on a clear screen.
        if b"\x1b[?1049h" in data:
            screen.reset()
            data = data[data.index(b"\x1b[?1049h") + 8 :]
        stream.feed(data)
        snapshot = [[(c.data, c.fg, c.bold) for c in (screen.buffer[y][x] for x in range(COLS))] for y in range(ROWS)]
        if not out or out[-1][1] != snapshot:
            out.append((at, snapshot))
        if ended:
            break
    return out


def draw(snapshot, font, cell):
    w, h = cell
    img = Image.new("RGB", (COLS * w + 2 * w, ROWS * h + 2 * w), BACKGROUND)
    d = ImageDraw.Draw(img)
    for y, row in enumerate(snapshot):
        for x, (ch, fg, bold) in enumerate(row):
            if ch in (" ", ""):
                continue
            color = PALETTE.get(fg, PALETTE["default"])
            d.text((w + x * w, w + y * h), ch, font=font, fill=color)
            if bold:
                d.text((w + x * w + 1, w + y * h), ch, font=font, fill=color)
    return img


def main():
    font = ImageFont.truetype(FONT, SIZE)
    cell = (round(font.getlength("M")), round(SIZE * 1.3))
    shots = screens(record())
    if not shots:
        sys.exit("nothing was recorded")
    frames, durations = [], []
    for i, (at, snap) in enumerate(shots):
        nxt = shots[i + 1][0] if i + 1 < len(shots) else at + 2.5  # hold the end
        ms = int((nxt - at) * 1000)
        if ms < 40 and i + 1 < len(shots):
            continue  # a screen half drawn
        frames.append(draw(snap, font, cell).convert("P", palette=Image.ADAPTIVE, colors=32))
        durations.append(max(ms, 40))
    frames[0].save("../demo.gif", save_all=True, append_images=frames[1:], duration=durations, loop=0, optimize=True)
    print(f"{len(frames)} frames, {sum(durations) / 1000:.1f}s")


if __name__ == "__main__":
    main()
