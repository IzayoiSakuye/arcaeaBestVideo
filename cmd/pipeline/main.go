package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/job"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/tools"
)

// flags 参数初始化
var (
	project = flag.String("project", "samples/sample-project.json", "输入样例")
	out     = flag.String("out", "dist/sample/sample.mp4", "成片输出")
	root    = flag.String("root", ".", "项目根目录")
	dataDir = flag.String("data", "data", "数据包根目录")
	workDir = flag.String("work", ".cache/pipeline", "渲染工作区")
	clips   = flag.String("clips", ".cache/clips", "录像片段缓存目录")
	force   = flag.Bool("force", false, "已存在的分段也重渲")
	limit   = flag.Int("limit", 0, "只处理前 N 首（0 = 全部）")
)

func main() {
	flag.Parse()
	if flag.NArg() >= 1 {
		log.Fatal(fmt.Errorf("unknown argument: %s", flag.Args()))
	}

	abs, err := filepath.Abs(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	cata, err := catalog.Load(filepath.Join(abs, "arcaea-catalog.json"), 1)
	if err != nil {
		log.Fatal(fmt.Errorf("catalog load failed: %w", err))
	}
	bgm, err := resource.LoadBgManifest(filepath.Join(abs, "arcaea-backgrounds.json"))
	if err != nil {
		log.Fatal(fmt.Errorf("bg load failed: %w", err))
	}
	tl, err := tools.Resolve(tools.Detect(tools.Tools{}))
	if err != nil {
		log.Fatal(fmt.Errorf("tools resolve failed: %w", err))
	}

	// CLI 只把事件打成日志；App 里换成 runtime.EventsEmit，编排本身不用动。
	j := job.NewJob(
		job.Deps{Catalog: cata, Bg: bgm, Tools: tl},
		job.Config{
			ProjectPath: *project,
			Output:      *out,
			Root:        *root,
			DataDir:     *dataDir,
			WorkDir:     *workDir,
			ClipsDir:    *clips,
			Force:       *force,
			Limit:       *limit,
		},
		func(ev job.Event) {
			if ev.Status == job.StatusSkipped {
				log.Printf("skipping song %s", ev.SongID)
			}
		},
	)

	res, err := j.Run(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	printPhases(res)
}

// printPhases 汇总分阶段耗时。格式沿用 P1，别改宽度（AGENTS.md / docs 的耗时表按它对口径）。
func printPhases(res job.Result) {
	if res.Rendered == 0 {
		log.Printf("全部命中缓存，没有渲染耗时可报")
		return
	}

	log.Printf("分阶段耗时（%d 段）：", res.Rendered)
	for _, p := range job.PhaseOrder {
		d := res.Phases[p]
		log.Printf("  %-11s %5.2fs/段   合计 %6.1fs   %3.0f%%",
			p, d.Seconds()/float64(res.Rendered), d.Seconds(), share(d, res.Phases))
	}
	log.Printf("  %-11s %11s   合计 %6.1fs", job.PhaseConcat, "—", res.Concat.Seconds())

	est := job.EstimateMinutes(res.Phases, res.Rendered, res.Concat)
	verdict := "达标"
	if est > 10 {
		verdict = fmt.Sprintf("超 %.1f 倍", est/10)
	}
	log.Printf("  单段合计      %5.2fs/段", job.PerSegment(res.Phases, res.Rendered).Seconds())
	log.Printf("  50 段外推 ≈ %.1f 分钟（预算 ≤10 分钟 → %s）", est, verdict)
}

// share 是某阶段占三个渲染阶段总和的百分比。总和为 0 时返回 0，避免亚毫秒阶段打出 NaN。
func share(d time.Duration, phases map[string]time.Duration) float64 {
	var total time.Duration
	for _, p := range job.PhaseOrder {
		total += phases[p]
	}
	if total == 0 {
		return 0
	}
	return float64(d) / float64(total) * 100
}
