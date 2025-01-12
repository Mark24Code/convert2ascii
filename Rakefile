# frozen_string_literal: true
require "bundler/gem_tasks"

desc "Run Test"
task :test do
  tests = Dir.glob("./test/test_*.rb")
  tests.each do |t|
    system("ruby #{t}")
  end
end


task default: %i[]
