package core

import (
	"fmt"
	"strings"
	"time"
)

func buildBossHeader(th theme) string {
	now := time.Now().Format("15:04:05")
	host := "edge-gw-03"
	job := "batch-reconcile"
	switch th.Name {
	case "jetbrains":
		line1 := fmt.Sprintf("[%s](fg:yellow,mod:bold)  workspace [%s](fg:cyan)  run [%s](fg:green)  branch [%s](fg:magenta)  [%s](fg:yellow)",
			th.HeaderName, host, job, "hotfix/runtime", now)
		line2 := "[RUNNING](fg:black,bg:yellow,mod:bold)  [ processes ] [ logs ] [ traces ] [ queues ] [ alerts ]  inspections [0](fg:green)  state [healthy](fg:cyan)"
		return line1 + "\n" + line2
	case "ops-console":
		line1 := fmt.Sprintf("[%s](fg:green,mod:bold)  node [%s](fg:cyan)  task [%s](fg:yellow)  window [%s](fg:white,mod:bold)  [%s](fg:green)",
			th.HeaderName, host, job, "runtime-monitor", now)
		line2 := "[ACTIVE](fg:black,bg:green,mod:bold)  [ ingest ] [ workers ] [ streams ] [ snapshots ] [ audit ]  incidents [0](fg:green)  state [stable](fg:cyan)"
		return line1 + "\n" + line2
	default:
		line1 := fmt.Sprintf("[%s](fg:cyan,mod:bold)  service [%s](fg:yellow)  env [%s](fg:green)  pod [%s](fg:white,mod:bold)  [%s](fg:cyan)",
			th.HeaderName, job, "prod-sh", host, now)
		line2 := "[LIVE](fg:black,bg:cyan,mod:bold)  [ overview ] [ jobs ] [ traces ] [ tasks ] [ deployment ]  diagnostics [0](fg:green)  state [synced](fg:yellow)"
		return line1 + "\n" + line2
	}
}

func buildBossLeftPanel() string {
	return strings.Join([]string{
		"[Process Tree](fg:cyan,mod:bold)",
		"",
		"  supervisor/",
		"    scheduler/",
		"      job-dispatcher",
		"      signal-watcher",
		"    workers/",
		"      parser-01",
		"      parser-02",
		"      merge-queue",
		"    network/",
		"      stream-relay",
		"      rpc-gateway",
		"",
		"[Jobs](fg:yellow,mod:bold)",
		"  pending      03",
		"  running      12",
		"  blocked      00",
		"  retrying     01",
		"",
		"[Focus](fg:green,mod:bold)",
		"  target  nightly sync",
		"  lane    cn-sh-prod",
		"  state   processing",
	}, "\n")
}

func buildBossMainPanel() string {
	now := time.Now()
	lines := []string{
		"[runtime monitor](fg:cyan,mod:bold)",
		"",
		fmt.Sprintf("[%s](fg:green)  bootstrap completed, worker pool online", now.Add(-29*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:green)  queue snapshot refreshed, 124 batches ready", now.Add(-24*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:yellow)  parser-01 processing shard eu-west/14", now.Add(-19*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:yellow)  parser-02 processing shard ap-east/09", now.Add(-16*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:cyan)  merge-queue flushing 18 delta segments", now.Add(-12*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:white)  rpc-gateway heartbeat stable (p95 42ms)", now.Add(-9*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:green)  audit channel synced, no drift detected", now.Add(-6*time.Second).Format("15:04:05")),
		fmt.Sprintf("[%s](fg:green)  release window healthy, checksum verified", now.Add(-3*time.Second).Format("15:04:05")),
		"",
		"[active tasks](fg:yellow,mod:bold)",
		"  batch-reconcile      running      73%",
		"  metrics-rollup       running      41%",
		"  cold-storage-sync    queued       wait-io",
		"  nightly-report       standby      00:12",
		"",
		"[stream output](fg:magenta,mod:bold)",
		"  shard/eu-west/14     rows 184220   lag 0.3s",
		"  shard/ap-east/09     rows 163871   lag 0.4s",
		"  shard/cn-south/02    rows 201443   lag 0.2s",
		"",
		"[controls](fg:green,mod:bold)",
		"  b  back to reader",
		"  q  exit to shelf",
	}
	return strings.Join(lines, "\n")
}

func buildBossRightPanel() string {
	return strings.Join([]string{
		"[Runtime](fg:cyan,mod:bold)",
		"",
		"  cpu        37%",
		"  memory     2.4 GB",
		"  io wait    1.2%",
		"  net        128 MB/s",
		"",
		"[Workers](fg:yellow,mod:bold)",
		"  online     12",
		"  idle       03",
		"  errors     00",
		"",
		"[Queue](fg:green,mod:bold)",
		"  inflight   124",
		"  retry      01",
		"  backlog    08",
		"",
		"[Recent](fg:magenta,mod:bold)",
		"  parser pool healthy",
		"  storage sync ready",
		"  monitor stable",
	}, "\n")
}

func buildBossFooter() string {
	elapsed := time.Since(app.sessionStart).Round(time.Minute)
	return fmt.Sprintf("[MONITOR](fg:black,bg:green,mod:bold)  uptime [%s](fg:yellow)  state [running](fg:green)  window [runtime](fg:cyan)  [%s](fg:white)\n[Escaped view active](fg:yellow)  [b](fg:cyan):return  [q](fg:red):shelf",
		elapsed, time.Now().Format("2006-01-02 15:04:05"))
}
