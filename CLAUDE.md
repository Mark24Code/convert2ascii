# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`convert2ascii` is a Ruby gem that renders images and videos as ASCII art in the terminal. Two executables are shipped: `image2ascii` and `video2ascii`. It depends on external binaries ImageMagick (via the `rmagick` gem) and `ffmpeg`; both are verified at runtime by the `CheckPackage` hierarchy before any conversion happens.

**Runtime prerequisites:** Ruby ≥ 3.1, ImageMagick (with MagickWand dev headers for rmagick), and ffmpeg. On macOS these are installed via `brew`; Linux support is per-distro (debian/redhat/arch).

## Commands

```bash
bundle install                          # install gem + dev deps
rake test                               # run all tests (globs ./test/test_*.rb)
ruby test/test_01_image2ascii.rb        # run a single test file
rake build                              # build the .gem (bundler/gem_tasks)
rake install                            # install the built gem locally
rake build_docker / push_docker / run_in_docker

# run the executables against the source tree (no install needed)
bundle exec exe/image2ascii -i path/to/image -w 80 -s text
bundle exec exe/video2ascii -i path/to/video.mp4 -w 80   # generate + play
bundle exec exe/video2ascii -i path/to/video.mp4 -o ./frames   # save frames only
bundle exec exe/video2ascii -p ./frames --loop           # play a saved frames dir
```

Tests are plain minitest files (`test/test_*.rb`) that run standalone with `require "minitest/autorun"` — `rake test` just shells out to `ruby` on each. The video tests are slow and exercise real ffmpeg slicing against `test/assets/fireworks.mp4`; `test_03` also spins up an interactive `TerminalPlayer`.

## Architecture

The core conversion is a two-stage pipeline: **frame extraction** (video only) → **per-frame ASCII rendering**, then optional **terminal playback**.

### Image2Ascii (`lib/convert2ascii/image2ascii.rb`) — the heart

Everything eventually funnels through this class. It:
1. Reads the image via `URI.open` (accepts local paths or URLs) into a `Magick::ImageList` blob.
2. Scales to `@width` and halves the height (`correct_aspect_ratio`) — terminal glyphs are ~2× taller than wide.
3. Walks every pixel, computing relative-luminance brightness `0.2126r + 0.7152g + 0.0722b`, and maps it onto the `@chars` ramp (darkest → brightest).
4. Emits either plain `text` chars or ANSI-colored output via the `rainbow` gem (`color` style; `color_block: true` swaps chars for colored background spaces).

`@quantum_convert_factor` normalizes ImageMagick's quantum depth (16-bit → 257, else 1) so RGB lands in 0–255; depths > 16 raise `Image2AsciiError`. Style/color enums live in `STYLE_ENUM` / `COLOR_ENUM` modules nested on the class. The class is stateful — `generate(**args)` mutates and returns `self` so it chains (`.generate.tty_print`), and `attr_reader :width, :ascii_string` / `attr_accessor :chars` expose knobs.

### Video2Ascii (`lib/convert2ascii/video2ascii.rb`) — orchestrator

Breaks a video into per-frame `.txt` ASCII files plus a `meta.json`, staging everything under `~/.convert2ascii` (a temp dir, wiped by `after_clean` after every operation):
1. `get_audio_from_video` — `ffmpeg -vn` → `audio.mp3` (nil if no audio track).
2. `screenshots_from_video` — `ffmpeg -vf fps=1/@step_duration` (default `0.04`s ≈ 25fps) → numbered `%d.jpg` frames.
3. `convert_all_images` — converts every jpg via `Image2Ascii` and writes `N.txt`; runs in **parallel subprocesses** through `MultiTasker`.
4. Writes `meta.json` (step_duration, audio filename, frames_count) so a saved frame dir is replayable later.

Frames are named by integer sequence and sorted numerically (`get_name_order` → `File.basename(..., ".*").to_i`) — never lexically, or frame 10 sorts before frame 2. Public API: `generate` (returns self), `save(output_dir)` (copies txts + audio out, strips the jpgs), `play` (hands frames to `TerminalPlayer`).

### MultiTasker (`lib/convert2ascii/multi-tasker.rb`) — parallel fan-out

Thin wrapper over the `parallel` gem: `Parallel.map(tasks, in_processes: count)` with a progress bar callback. Thread count is `Etc.nprocessors`, trimmed for high-core machines (subtracts 1 for Video2Ascii, 2 here) — tuned by the numbers in `benchmark/benchmark.txt`.

### TerminalPlayer (`lib/convert2ascii/terminal-player.rb`) — playback

Sequential frame renderer with **self-adaptive A/V sync** (`self_adaption_frame_play`). It compares real elapsed time against `frame_index * @step_duration` and adjusts the frame advance within a tolerance window: skip ahead if video lags audio by > `SAFE_SLOW_DELTA` (0.9s), pause if it runs > 0.2s ahead, else advance 1 frame. Audio plays in a background `Thread` via `ffplay -loop 0`; playback uses the ANSI alternate screen buffer (open/close via `Terminal` helpers), hides the cursor, and pads/truncates each frame to the terminal height with `full_screen`.

### Terminal (`lib/convert2ascii/terminal.rb`) — ANSI helpers

Class methods wrapping escape codes: winsize, clear screen/buffer, show/hide cursor, open/close the `?1049` alternate buffer.

### CheckPackage (`lib/convert2ascii/check_package.rb`) — runtime deps

`CheckPackage` base dispatches `#{os}_check` by `RUBY_PLATFORM` (`OS::detect_os`); subclasses `CheckFFmpeg` and `CheckImageMagick` implement per-OS install checks (brew list / dpkg / rpm / pacman / yum). Linux distro detection reads `/etc/os-release`.

## Conventions

- Executables in `exe/` use `OptionParser` and are plain `#!/usr/bin/env ruby` scripts; `image2ascii` and `video2ascii` are the only published binaries (`gemspec.bindir = "exe"`).
- File naming is inconsistent — `multi-tasker.rb` and `terminal-player.rb` use hyphens — so `require_relative` paths must be copied exactly.
- Both classes return `self` from `generate` to support fluent chaining.
- `after_clean` on Video2Ascii always removes `~/.convert2ascii`; rely on it rather than leaving artifacts.
- New executables/features should extend the existing enums (`STYLE_ENUM`, `COLOR_ENUM`) rather than introducing parallel option systems.

## Notes

- Current branch is `goversion` — a planned Go rewrite lives here but **no Go files exist yet**; the Ruby gem remains the working implementation.
- Windows is unsupported (README test matrix); Windows/mingw detection exists in `check_package.rb` but `ms_check` is a no-op.
- Known limits in `TODO.md`: playback lacks pause/next/prev/exit controls, and loading all frames into memory caps video length.
