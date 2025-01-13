require "parallel"
require "etc"
require "rainbow"

module Convert2Ascii
  class MultiTasker
    def initialize(proc_tasks)
      @proc_tasks = proc_tasks
      @count = set_threads_count
      @finished = []
      @time_start = nil
    end

    def set_threads_count
      cpu_threads = (Etc.nprocessors || 1)
      cpu_threads = cpu_threads > 4 ? cpu_threads - 2 : 1
      cpu_threads
    end

    def progress(index)
      @finished << index
      print(Rainbow("\rprocessing...  #{sprintf("%.2f", (1.0 * @finished.length / @proc_tasks.length) * 100)} % (time: #{sprintf("%.2f", Time.now - @time_start)} s)").green)
    end

    def run
      @time_start = Time.now
      results = Parallel.map(@proc_tasks, in_processes: @count, finish: ->(item, index, result) { progress(index) }, finish_in_order: true) do |task|
        task && task.call
      end

      results
    end
  end
end
