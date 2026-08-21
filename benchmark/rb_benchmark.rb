# frozen_string_literal: true

# Benchmark the Ruby implementation of convert2ascii against a single video.
#
# Usage:
#   ruby benchmark/rb_benchmark.rb <video_uri> [width]
#
# Measures each phase of the Convert2Ascii::Video2Ascii pipeline (audio extract,
# frame slicing, parallel ascii conversion) plus total wall-clock of #generate.

require "etc"
require "json"
require_relative "../lib/convert2ascii/video2ascii"

$stdout.sync = true # avoid forked workers (Parallel) flushing inherited buffers

# Headless-safe fallback. Image2Ascii defaults its width from IO.console.winsize,
# which returns nil when stdout is redirected / no controlling tty, making every
# frame conversion raise ("undefined method `winsize' for nil"). The width is
# always overridden by the explicit width, so this only fixes the default.
if IO.respond_to?(:console) && IO.console.nil?
  FakeConsole = Struct.new(:winsize)
  def IO.console
    FakeConsole.new([40, 80])
  end
end

# Subclass that wraps the private pipeline steps with monotonic-clock timing,
# so we can report a per-phase breakdown of the real production code path.
class TimedVideo2Ascii < Convert2Ascii::Video2Ascii
  attr_reader :phase_times, :frame_count

  def initialize(**args)
    super
    @phase_times = {}
    @frame_count = 0
  end

  def get_audio_from_video(save_dir)
    measure(:audio_extract) { super }
  end

  def screenshots_from_video(save_dir)
    measure(:frame_slice) { super }
  end

  def convert_all_images(save_dir)
    measure(:ascii_convert) { super }
  end

  def order_frames_path
    result = super
    @frame_count = @frames_path.length
    result
  end

  private

  def measure(key)
    t0 = Process.clock_gettime(Process::CLOCK_MONOTONIC)
    result = yield
    @phase_times[key] = Process.clock_gettime(Process::CLOCK_MONOTONIC) - t0
    result
  end
end

# Swallow the "processing... N%" progress bar (goes to stdout during generate)
# so the report file stays clean.
def silence_stdout
  old = $stdout
  $stdout = File.open(File::NULL, "w")
  yield
ensure
  $stdout = old
end

uri = ARGV[0] || "./videos/demo.mp4"
width = (ARGV[1] || 80).to_i

probe = JSON.parse(`ffprobe -v error -select_streams v:0 -show_entries stream=width,height,r_frame_rate,nb_frames -show_entries format=duration,bit_rate -of json "#{uri}"`)
stream = probe["streams"]&.first || {}
format = probe["format"] || {}
w, h = stream["width"], stream["height"]
fps = stream["r_frame_rate"].to_s.split("/").first.to_i
nb_frames = stream["nb_frames"].to_i
duration = format["duration"].to_f
bytes = File.size(uri)

puts "=" * 60
puts "convert2ascii (Ruby) benchmark"
puts "=" * 60
puts format("%-16s : %s", "machine", `sysctl -n machdep.cpu.brand_string 2>/dev/null`.strip)
puts format("%-16s : %d CPUs (Etc.nprocessors=%d)", "machine", Etc.nprocessors, Etc.nprocessors)
puts format("%-16s : %s", "ruby", RUBY_VERSION)
puts format("%-16s : v%s", "gem version", Convert2Ascii::VERSION)
puts ""
puts format("%-16s : %s", "video", uri)
puts format("%-16s : %dx%d", "resolution", w, h)
puts format("%-16s : %d fps, %d frames, %.2f s", "stream", fps, nb_frames, duration)
puts format("%-16s : %.2f MB", "size", bytes / 1024.0 / 1024.0)
puts ""
puts format("%-16s : %d", "width", width)
puts format("%-16s : %.2f s (%.0f fps)", "step_duration", Convert2Ascii::Video2Ascii::DEFAULT_STEP_DURATION, 1.0 / Convert2Ascii::Video2Ascii::DEFAULT_STEP_DURATION)
puts ""

engine = TimedVideo2Ascii.new(uri: uri, width: width)
puts format("%-16s : %d", "slice threads", engine.threads_count)
puts ""

t_total = Process.clock_gettime(Process::CLOCK_MONOTONIC)
silence_stdout { engine.generate }
total = Process.clock_gettime(Process::CLOCK_MONOTONIC) - t_total

audio = engine.phase_times[:audio_extract] || 0.0
slice = engine.phase_times[:frame_slice] || 0.0
conv = engine.phase_times[:ascii_convert] || 0.0
sum = audio + slice + conv
meta = total - sum

puts "phases (wall clock):"
puts format("  %-14s : %8.3f s", "audio_extract", audio)
puts format("  %-14s : %8.3f s", "frame_slice", slice)
puts format("  %-14s : %8.3f s  (%d frames, %.1f frames/s)", "ascii_convert", conv, engine.frame_count, engine.frame_count / conv)
puts format("  %-14s : %8.3f s  (meta.json + overhead)", "meta/other", meta)
puts format("  %-14s : %8.3f s", "TOTAL generate", total)
puts ""
expected = duration / Convert2Ascii::Video2Ascii::DEFAULT_STEP_DURATION
puts format("expected frames @ %.0f fps: %.0f | actual: %d", 1.0 / Convert2Ascii::Video2Ascii::DEFAULT_STEP_DURATION, expected, engine.frame_count)
puts ""
puts "(user/sys time & peak RSS are reported by the outer /usr/bin/time -l)"
puts "(headless run: IO.console stubbed; total conversion time is the PK metric)"

engine.after_clean
