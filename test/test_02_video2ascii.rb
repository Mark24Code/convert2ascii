require "minitest/autorun"
require_relative '../lib/convert2ascii/video2ascii'

class TestDefaultParams < Minitest::Test
  def setup
    @width = 20
    @fake_uri = "./test/assets/fireworks.mp4"
    @generator = Convert2Ascii::Video2Ascii.new(uri: @fake_uri, width: @width)
  end

  def test_initialize_params_width
    assert_equal @width, @generator.width
  end

  def test_initialize_params_duration
    assert_equal Convert2Ascii::Video2Ascii::DEFAULT_STEP_DURATION, @generator.step_duration
  end

  def test_generate_params
    test_width = 200
    assert_equal test_width , @generator.generate(width: test_width).width
  end

  def test_generate_params
    test_width = 200
    assert_equal test_width , @generator.generate(width: test_width).width
  end

  def teardown
    @generator.after_clean
  end
end
