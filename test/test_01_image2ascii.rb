require "minitest/autorun"
require_relative '../lib/convert2ascii/image2ascii'

class TestDefaultParams < Minitest::Test
  def setup
    @width = 20
    @fake_uri = "./test/assets/ruby.jpg"
    @generator = Convert2Ascii::Image2Ascii.new(uri:@fake_uri, width: @width)
  end

  def test_initialize_params
    assert_equal @width, @generator.width
  end

  def test_chars_reader
    assert @generator.chars.length > 0
  end

  def test_chars_writer
    chars = "abc"
    @generator.chars = chars
    assert_equal chars,  @generator.chars
  end
end


class TestGenerate < Minitest::Test
  def setup
    @width = 20
    @fake_uri = "./test/assets/ruby.jpg"
    @generator = Convert2Ascii::Image2Ascii.new(uri: @fake_uri)
  end

  def test_generate_params
    @generator.generate(width: @width)
    assert_equal @width, @generator.width
  end


  def test_generate_style_color
    gen = @generator.generate(width: @width, style: Convert2Ascii::Image2Ascii::STYLE_ENUM::Color)
    ascii_string = gen.ascii_string
    print ascii_string
    assert ascii_string.length > 0 && gen.ascii_string != ""
  end

  def test_generate_style_color_block
    gen = @generator.generate(width: @width, style: Convert2Ascii::Image2Ascii::STYLE_ENUM::Color, color_block: true)
    ascii_string = gen.ascii_string
    print ascii_string
    assert ascii_string.length > 0 && gen.ascii_string != ""
  end

  def test_generate_style_text
    gen = @generator.generate(width: @width, style: Convert2Ascii::Image2Ascii::STYLE_ENUM::Text)
    ascii_string = gen.ascii_string
    print ascii_string
    assert ascii_string.length > 0 && gen.ascii_string != ""
  end
end
