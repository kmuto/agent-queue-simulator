#!/usr/bin/env ruby
# Copyright 2023 Kenshi Muto
# mackerel-agentのキューがどう処理されるかのシミュレーション
# PostMetricIntervalは60秒のままでの想定
require 'time'
require 'pastel'
require 'yaml'

config = YAML.load_file('config.yml')
pastel = Pastel.new
# キュー
queue = []
# {name: 'M1', started: 0, retry: 0 }, ...

starttime = Time.parse(config['during'][0])

# duringから実行分を取得
running_minutes = (Time.parse(config['during'][1]) - starttime).to_i / 60

# accidentsからトラブル時間帯をsparse配列で記録
timeline = []
config['accidents'].each_with_index do |fromtill, i|
  # 障害開始
  from = (Time.parse(fromtill[0]) - starttime).to_i / 60
  till = (Time.parse(fromtill[1]) - starttime).to_i / 60
  from.upto(till - 1).each do |minute|
    timeline[minute] = "accident #{i+1}"
  end
end

mode = :normal
accidentmode = nil

def log(message)
  puts "#{@attime} #{message}"
end

losts = []

0.upto(running_minutes) do |progress_minutes|
  @attime = (starttime + progress_minutes * 60).strftime('%H:%M')
  # 30秒ずつの表現
  0.upto(1) do |halfmin|
    if progress_minutes > 0 && halfmin == 0
      # 1分ごとに新たなメトリックがやってくる
      metric = {name: "M#{progress_minutes}", started: @attime, retry: 0}
      queue.push(metric)
    end
    if queue.size > config['buffersize']
      log(pastel.magenta.bold('@ reached queue limit'))
      # あきらめはこれでいい？
      q = queue.shift  # 最初を取る
      log(pastel.red.bold("@ limit lost #{q}"))
      losts.push(q)
    end

    nextmode = if queue.size > 0
      :queued
    else
      :default
    end

    # queuedだったら30秒ごと、それ以外は1分ごと
    if (mode == :default || mode == :haderror) && halfmin == 1
      mode = nextmode
      next
    end
    mode = nextmode

    if timeline[progress_minutes] && accidentmode == nil
      # 障害
      log(pastel.magenta.bold("@ #{timeline[progress_minutes]} happened"))
      accidentmode = true
    elsif !timeline[progress_minutes] && accidentmode == true
      # 復帰
      log(pastel.blue.bold("@ #{timeline[progress_minutes - 1]} recovered (#{queue.size} posts remained)"))
      accidentmode = nil
    end

    if queue.size > 0
      q1 = [queue.shift] # 最初を取る
      if queue.size > 0
        q1.push(queue.shift) # もう1つ取る
      end

      # 投稿
      if accidentmode.nil?
        if q1.size > 1
          log(pastel.green.bold("@ posted with recover #{q1}"))
        else
          log(pastel.cyan("posted #{q1}")) unless config['quiet']
        end
      else
        # 障害で送れなかった
        mode = :haderror
        q1.each do |metric|
          metric[:retry] += 1
          if metric[:retry] > config['max_retry']
            log(pastel.red.bold("@ retry lost #{metric}"))
            losts.push(metric)
            next
          end
          queue.push(metric)
        end
      end
    end
  end
end
if queue.size > 0
  log(pastel.red.bold("@ #{queue.size} posts remained"))
  queue.sort {|a, b| a[:started] <=> b[:started] }.each do |metric|
    log(pastel.blue("@ remain #{metric}"))
  end
end

losts.sort {|a, b| a[:started] <=> b[:started] }.each do |metric|
    log(pastel.red.bold("@ finally lost #{metric}"))
end
