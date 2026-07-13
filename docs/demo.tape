# Regenerate the README GIF:  vhs docs/demo.tape  ->  docs/akuaku.gif
# Builds akuaku from the repo, then records the `akuaku demo` fleet for one loop.

Output docs/akuaku.gif

Set Shell "bash"
Set FontSize 16
Set Width 1100
Set Height 500
Set Padding 20
Set Theme "Catppuccin Mocha"
Set Framerate 12

# Build and put akuaku on PATH off-camera, so the recording opens on a clean prompt.
Hide
Type `make build && export PATH="$PWD/bin:$PATH" && clear`
Enter
Sleep 5s
Show

Type "akuaku demo"
Sleep 800ms
Enter
Sleep 15s
Type "q"
Sleep 400ms
