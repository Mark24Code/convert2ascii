require "minitest/autorun"
require_relative '../lib/convert2ascii/video2ascii'

class Video2AsciiPlay < Minitest::Test
  def setup
    @width = 20
    @fake_uri = "./test/assets/fireworks.mp4"
    @generator = Convert2Ascii::Video2Ascii.new(uri:@fake_uri, width: @width)
  end

  def test_video_play
    @generator.generate
    assert_equal true , @generator.play
  end

  def teardown
    @generator.after_clean
  end
end
