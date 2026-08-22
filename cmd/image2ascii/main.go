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

const imageUsage = `Usage: image2ascii [options]
        --version                    version
    -i, --image=URI                  image uri (required)
    -w, --width=WIDTH                image width (integer)
    -s, --style=STYLE                ascii style: 'color'/'text'
    -b, --block                      ascii color style use BLOCK or not true/false
`

type imageArgs struct {
	image, style         string
	width                int
	block, version, help bool
}

func parseImageArgs(args []string) (imageArgs, error) {
	var a imageArgs
	fs := flag.NewFlagSet("image2ascii", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	fs.BoolVar(&a.version, "version", false, "version")
	fs.StringVar(&a.image, "image", "", "image uri")
	fs.StringVar(&a.image, "i", "", "image uri (short)")
	fs.IntVar(&a.width, "width", 0, "image width")
	fs.IntVar(&a.width, "w", 0, "image width (short)")
	fs.StringVar(&a.style, "style", "", "ascii style")
	fs.StringVar(&a.style, "s", "", "ascii style (short)")
	fs.BoolVar(&a.block, "block", false, "color block")
	fs.BoolVar(&a.block, "b", false, "color block (short)")
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
	a, err := parseImageArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprint(os.Stderr, imageUsage)
		os.Exit(1)
	}
	if a.version {
		fmt.Printf("convert2ascii/image2ascii: v%s\n", version.Version)
		fmt.Printf("author: %s\n", version.Author)
		fmt.Printf("mail: %s\n", version.Mail)
		fmt.Printf("project: %s\n", version.Project)
		return
	}
	if a.help {
		fmt.Print(imageUsage)
		return
	}
	if a.image == "" {
		fmt.Println("Error: --image option is required.")
		os.Exit(1)
	}
	if a.style != "" && a.style != "color" && a.style != "text" {
		fmt.Println("Error: --style option must be [\"color\" | \"text\"].")
		os.Exit(1)
	}

	opts := convert2ascii.ImageOptions{
		URI:        a.image,
		Width:      a.width,
		Style:      a.style,
		ColorBlock: a.block,
	}
	img := convert2ascii.NewImage2Ascii(opts)
	if err := img.Generate(); err != nil {
		fmt.Fprintf(os.Stderr, "[Error] %v\n", err)
		os.Exit(1)
	}
	fmt.Print(img.String())
}
