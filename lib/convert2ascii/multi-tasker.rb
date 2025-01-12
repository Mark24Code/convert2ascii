require "async"
require "async/barrier"
require "async/semaphore"
require "etc"

module Convert2Ascii
  class MultiTasker
    def initialize(proc_tasks)
      @proc_tasks = proc_tasks
      @count = set_threads_count
    end

    def set_threads_count
      cpu_threads = (Etc.nprocessors || 1)
      cpu_threads = cpu_threads > 4 ? cpu_threads - 1 : cpu_threads
      cpu_threads
    end

    def run
      barrier = Async::Barrier.new

      Sync do
        # Only 10 tasks are created at a time:
        semaphore = Async::Semaphore.new(@count, parent: barrier)

        @proc_tasks.map do |proc_task|
          semaphore.async do
            proc_task.call
          end
        end.map(&:wait)
      ensure
        barrier.stop
      end
    end
  end
end
