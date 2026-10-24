package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"rhythm-game-bvideo/internal/tools"
)

// SheetReader 调用随应用提供的本地文字识别程序
type SheetReader struct {
	Program  string
	Args     []string
	ModelDir string
}

// FindSheetReader 优先使用发布包，开发时使用隔离环境
func FindSheetReader(root string) (SheetReader, error) {
	if root == "" {
		root = "."
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return SheetReader{}, err
	}
	for _, dir := range []string{filepath.Join(root, "ocr"), filepath.Join(root, "build", "bin", "ocr")} {
		program := filepath.Join(dir, "recognize.exe")
		model := filepath.Join(dir, "model")
		if validReaderFiles(program, model) {
			return SheetReader{Program: program, ModelDir: model}, nil
		}
	}
	program := filepath.Join(root, ".cache", "ocr-venv", "Scripts", "python.exe")
	script := filepath.Join(root, "sidecar", "ocr", "recognize.py")
	model := filepath.Join(root, ".cache", "ocr-onnx-model")
	if validReaderFiles(program, model) {
		if _, err := os.Stat(script); err == nil {
			return SheetReader{Program: program, Args: []string{script}, ModelDir: model}, nil
		}
	}
	return SheetReader{}, fmt.Errorf("未找到文字识别组件；请构建 OCR 组件后再导入图片")
}

func validReaderFiles(program, model string) bool {
	if _, err := os.Stat(program); err != nil {
		return false
	}
	for _, name := range []string{"inference.onnx", "inference.yml"} {
		if _, err := os.Stat(filepath.Join(model, name)); err != nil {
			return false
		}
	}
	return true
}

// SheetText 保存图片上的文字和字段识别结果
type SheetText struct {
	Fields []Fields
	Titles []string
	Header Header
}

// ReadSheet 一次启动识别程序处理整张图的文字行
func (r SheetReader) ReadSheet(ctx context.Context, path string, cells []Cell) (*SheetText, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	request := struct {
		Image string   `json:"image"`
		Boxes [][4]int `json:"boxes"`
	}{Image: path, Boxes: make([][4]int, 0, len(cells)*4+3)}
	for _, line := range [][2]int{LineScore, LinePotential, LineJudgment, LineTitle} {
		for _, cell := range cells {
			box := LineBox(cell, line)
			if line == LineJudgment {
				box.Max.X = cell.Box.Min.X + 290
			}
			request.Boxes = append(request.Boxes, [4]int{box.Min.X, box.Min.Y, box.Max.X, box.Max.Y})
		}
	}
	for _, box := range []image.Rectangle{headerTop10, headerMax, headerTop50} {
		request.Boxes = append(request.Boxes, [4]int{box.Min.X, box.Min.Y, box.Max.X, box.Max.Y})
	}
	input, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resultFile, err := os.CreateTemp("", "b50-text-*.json")
	if err != nil {
		return nil, fmt.Errorf("创建 OCR 结果文件失败：%w", err)
	}
	resultPath := resultFile.Name()
	resultFile.Close()
	defer os.Remove(resultPath)
	args := append(append([]string{}, r.Args...), r.ModelDir, resultPath)
	cmd := exec.CommandContext(ctx, r.Program, args...)
	tools.HideWindow(cmd)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("图片文字识别失败：%w：%s", err, strings.TrimSpace(stderr.String()))
	}
	outputBytes, err := os.ReadFile(resultPath)
	if err != nil {
		return nil, fmt.Errorf("读取图片文字识别结果失败：%w", err)
	}
	var output []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(outputBytes, &output); err != nil {
		return nil, fmt.Errorf("图片文字识别结果无效：%w", err)
	}
	if len(output) != len(request.Boxes) {
		return nil, fmt.Errorf("图片文字识别返回 %d 行，预期 %d 行", len(output), len(request.Boxes))
	}
	result := &SheetText{Fields: make([]Fields, len(cells)), Titles: make([]string, len(cells))}
	n := len(cells)
	for i := range cells {
		f := parseScoreText(output[i].Text)
		p := parsePotentialText(output[n+i].Text)
		j := parseJudgmentText(output[2*n+i].Text)
		f.Constant, f.PTT, f.ClearType = p.Constant, p.PTT, p.ClearType
		f.Pure, f.ShinyPerfect, f.Far, f.Lost = j.Pure, j.ShinyPerfect, j.Far, j.Lost
		f.UnreadJudgments = j.UnreadJudgments
		f.Problems = append(f.Problems, p.Problems...)
		f.Problems = append(f.Problems, j.Problems...)
		result.Fields[i] = f
		result.Titles[i] = strings.TrimSpace(rankPattern.ReplaceAllString(output[3*n+i].Text, ""))
	}
	result.Header = parseHeaderText(output[4*n].Text, output[4*n+1].Text, output[4*n+2].Text)
	return result, nil
}

var (
	rankPattern     = regexp.MustCompile(`\s*#\d+$`)
	constantPattern = regexp.MustCompile(`(?i)Potential\s*(\d{1,2}[.,]\d)\s*>`)
	pttPattern      = regexp.MustCompile(`>\s*(\d{1,2}[.,]\d{4})`)
	clearPattern    = regexp.MustCompile(`(?i)\b(NC|HC|FR|PM)\s*$`)
	headerPattern   = regexp.MustCompile(`^\d{1,2}[.,]\d{5}$`)
	purePattern     = regexp.MustCompile(`P\s*[:：.·]?\s*(\d{1,4})(?:\D|$)`)
	shinyPattern    = regexp.MustCompile(`\(\s*\+\s*(\d{1,4})\s*\)`)
	farPattern      = regexp.MustCompile(`F\s*[:：.·]?\s*(\d{1,3})(?:\D|$)`)
	lostPattern     = regexp.MustCompile(`L\s*[:：.·]?\s*(\d{1,3})(?:\D|$)`)
)

func parseScoreText(s string) Fields {
	var f Fields
	s = strings.NewReplacer("’", "'", ",", "'", " ", "").Replace(strings.TrimSpace(s))
	digits := strings.ReplaceAll(s, "'", "")
	v, err := strconv.Atoi(digits)
	if len(digits) == 8 && err == nil && v >= 0 && v <= 11000000 {
		f.Score = v
	} else {
		f.Problems = append(f.Problems, "分数未能识别")
	}
	return f
}

func parsePotentialText(s string) Fields {
	var f Fields
	read := func(pattern *regexp.Regexp) (float64, bool) {
		match := pattern.FindStringSubmatch(s)
		if len(match) != 2 {
			return 0, false
		}
		v, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", "."), 64)
		return v, err == nil
	}
	if v, ok := read(constantPattern); ok {
		f.Constant = v
	} else {
		f.Problems = append(f.Problems, "定数未能识别")
	}
	if v, ok := read(pttPattern); ok {
		f.PTT = v
	} else {
		f.Problems = append(f.Problems, "单曲ptt未能识别")
	}
	if match := clearPattern.FindStringSubmatch(s); len(match) == 2 {
		f.ClearType = strings.ToUpper(match[1])
	} else {
		f.Problems = append(f.Problems, "通关类型未能识别")
	}
	return f
}

func parseHeaderText(top10, maxPotential, top50 string) Header {
	var h Header
	for _, item := range []struct {
		name string
		text string
		to   *float64
	}{
		{"TOP10 平均", top10, &h.Top10Avg},
		{"MAX POTENTIAL", maxPotential, &h.MaxPotential},
		{"TOP50 平均", top50, &h.Top50Avg},
	} {
		s := strings.TrimSpace(item.text)
		if !headerPattern.MatchString(s) {
			h.Problems = append(h.Problems, item.name+"未能识别")
			continue
		}
		v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
		if err != nil {
			h.Problems = append(h.Problems, item.name+"未能识别")
			continue
		}
		*item.to = v
	}
	return h
}

func parseJudgmentText(s string) Fields {
	var f Fields
	read := func(pattern *regexp.Regexp, max int) (int, bool) {
		match := pattern.FindStringSubmatch(s)
		if len(match) != 2 {
			return 0, false
		}
		value, err := strconv.Atoi(match[1])
		return value, err == nil && value <= max
	}
	for _, item := range []struct {
		name    string
		pattern *regexp.Regexp
		max     int
		to      *int
	}{
		{"PURE", purePattern, 9999, &f.Pure},
		{"FAR", farPattern, 999, &f.Far},
		{"LOST", lostPattern, 999, &f.Lost},
	} {
		if value, ok := read(item.pattern, item.max); ok {
			*item.to = value
		} else {
			f.Problems = append(f.Problems, item.name+"未能识别")
			f.UnreadJudgments = append(f.UnreadJudgments, strings.ToLower(item.name))
		}
	}
	if value, ok := read(shinyPattern, 2000); ok && (f.Pure == 0 || value <= f.Pure) {
		f.ShinyPerfect = &value
	} else {
		f.Problems = append(f.Problems, "大 P 数未能识别")
	}
	return f
}
