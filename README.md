# Convert2ASCII

Convert Image/Video to ASCII art.


## Prerequisites

* Ruby3+
* ImageMagick ([Download here](https://imagemagick.org/script/download.php))
* ffmpeg ([Download here](https://www.ffmpeg.org/))


## Executable command

### image2ascii

Make image to ascii art in your terminal.

```bash
image2ascii -h
Usage: image2ascii [options]
        --version                    verison
    -i, --image=URI                  image uri (required)
    -w, --width=WIDTH                image width (integer)
    -s, --style=STYLE                ascii style: ['color'| 'text']
    -b, --block                      ascii color style use BLOCK or not [ true | false ]
```

### video2ascii

Make image to ascii art in your terminal.

```bash
Usage: video2ascii [options]

* default will generate and play without save.
* -p will just play ascii frames dir, and ignore -i, -o others options. --loop will play loop
* -i,-o will just generate and output frames and ignore others options
        --version                    verison
    -i, --input=URI                  video uri (required)
    -w, --width=WIDTH                video width (integer)
    -s, --style=STYLE                ascii style: ['color'| 'text']
    -b, --block                      ascii color style use BLOCK or not [ true | false ]
    -o, --ouput=OUTPUT               save ascii frame to output dirname
    -p, --play_dir=PLAY_DIRNAME      input ascii frames dirname to play
        --loop
```


## As a Gem

### Convert2Ascii::Image2Ascii


```ruby
require 'convert2ascii/image2ascii'

# generate image
uri = "path/to/image"
ascii = Convert2Ascii::Image2Ascii.new(uri:, width: 50)

# generate image
ascii.generate
# display in your terminal
ascii.tty_print


# also chain call
ascii.generate.tty_print

```


### Convert2Ascii::Video2Ascii

```ruby
require 'convert2ascii/video2ascii'

# generate video
uri = "path/to/video.mp4"
ascii = Convert2Ascii::Video2Ascii.new(uri:, width: 50)
# generate video
ascii.generate
# save frames
ascii.save(output_path)

# play in terminal
ascii.play


# chain call
ascii.generate.play

```

## Inspired by

* [michaelkofron/image2ascii](https://github.com/michaelkofron/image2ascii)
* [andrewcohen/video_to_ascii](https://github.com/andrewcohen/video_to_ascii)
