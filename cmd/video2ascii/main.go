package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Mark24Code/convert2ascii/go2ascii"
	"github.com/Mark24Code/convert2ascii/go2ascii/internal/version"
)

const videoUsage = `Usage: video2ascii [options]

* default will generate and play without save.
* -p will just play ascii frames dir, and ignore -i, -o other options. --loop will play loop
* -i,-o will just generate and output frames and ignore others options
        --version                    version
    -i, --input=URI                  video uri (required)
    -w, --width=WIDTH                video width (integer)
    -s, --style=STYLE                ascii style: ['color'| 'text']
    -b, --block                      ascii color style use BLOCK or not [ true | false ]
    -o, --output=OUTPUT              save ascii frames to the output directory
    -p, --play_dir=PLAY_DIRNAME      input the ascii frames directory to play
        --loop
`

type videoArgs struct {
	input, style, output, playDir string
	width                         int
	block, loop, version, help    bool
}

func parseVideoArgs(args []string) (videoArgs, error) {
	var a videoArgs
	fs := flag.NewFlagSet("video2ascii", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	fs.BoolVar(&a.version, "version", false, "version")
	fs.StringVar(&a.input, "input", "", "video uri")
	fs.StringVar(&a.input, "i", "", "video uri (short)")
	fs.IntVar(&a.width, "width", 0, "video width")
	fs.IntVar(&a.width, "w", 0, "video width (short)")
	fs.StringVar(&a.style, "style", "", "ascii style")
	fs.StringVar(&a.style, "s", "", "ascii style (short)")
	fs.BoolVar(&a.block, "block", false, "color block")
	fs.BoolVar(&a.block, "b", false, "color block (short)")
	fs.StringVar(&a.output, "output", "", "output directory")
	fs.StringVar(&a.output, "o", "", "output directory (short)")
	fs.StringVar(&a.playDir, "play_dir", "", "play frames directory")
	fs.StringVar(&a.playDir, "p", "", "play frames directory (short)")
	fs.BoolVar(&a.loop, "loop", false, "play loop")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			a.help = true
			return a, nil
		}
		return a, err
	}
	return a, nil
}

func main() {
	a, err := parseVideoArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprint(os.Stderr, videoUsage)
		os.Exit(1)
	}
	if a.version {
		fmt.Printf("convert2ascii/video2ascii: v%s\n", version.Version)
		fmt.Printf("author: %s\n", version.Author)
		fmt.Printf("mail: %s\n", version.Mail)
		fmt.Printf("project: %s\n", version.Project)
		return
	}
	if a.help {
		fmt.Print(videoUsage)
		return
	}

	// -p replays a saved frames dir and ignores -i/-o.
	if a.playDir != "" {
		if err := convert2ascii.PlayFrames(a.playDir, a.loop); err != nil {
			fmt.Fprintf(os.Stderr, "[Error] %v\n", err)
			os.Exit(1)
		}
		return
	}
	if a.input == "" {
		fmt.Println("Error: --input option is required.")
		os.Exit(1)
	}
	if a.style != "" && a.style != "color" && a.style != "text" {
		fmt.Println("Error: --style option must be [\"color\" | \"text\"].")
		os.Exit(1)
	}

	opts := convert2ascii.VideoOptions{
		URI:        a.input,
		Output:     a.output,
		Style:      a.style,
		ColorBlock: a.block,
		Width:      a.width,
	}
	v := convert2ascii.NewVideo2Ascii(opts)
	if err := v.Generate(); err != nil {
		fmt.Fprintf(os.Stderr, "[Error] %v\n", err)
		os.Exit(1)
	}
	if a.output != "" {
		return
	}
	if err := v.Play(a.loop); err != nil {
		fmt.Fprintf(os.Stderr, "[Error] %v\n", err)
		os.Exit(1)
	}
}
