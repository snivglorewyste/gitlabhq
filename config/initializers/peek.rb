Rails.application.config.peek.adapter = :redis, { client: ::Redis.new(Gitlab::Redis.params) }

Peek.into Peek::Views::Host
Peek.into Peek::Views::PerformanceBar
Peek.into Gitlab::Database.mysql? ? Peek::Views::Mysql2 : Peek::Views::PG
Peek.into Peek::Views::Redis
Peek.into Peek::Views::Sidekiq
Peek.into Peek::Views::Rblineprof
Peek.into Peek::Views::GC

if Gitlab::Database.mysql?
  class Mysql2::Client
    class << self
      attr_accessor :query_details
    end
    self.query_details = Concurrent::Array.new
  end

  module Peek
    module Views
      class Mysql2 < View
        prepend ::Gitlab::PerformanceBar::PeekMysqlWithQueries
      end
    end
  end
else
  class PG::Connection
    class << self
      attr_accessor :query_details
    end
    self.query_details = Concurrent::Array.new
  end

  module Peek
    module Views
      class PG < View
        prepend ::Gitlab::PerformanceBar::PeekPgWithQueries
      end
    end
  end
end
