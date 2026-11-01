package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"rhythm-game-bvideo/internal/tools"
)

type AppStatus struct {
	DataDir     string         `json:"data_dir"`
	DataVersion string         `json:"data_version"`
	SongCount   int            `json:"song_count"`
	DataError   string         `json:"data_error,omitempty"`
	Tools       []tools.Status `json:"tools"`
	ToolError   string         `json:"tool_error,omitempty"`
}

// Status 汇总数据状态和工具检测结果
func (a *App) Status() AppStatus {
	st := AppStatus{DataDir: filepath.Join(a.dataRoot(), "data"), DataVersion: a.dataVersion, Tools: []tools.Status{}}
	if cata, err := a.catalog(); err != nil {
		st.DataError = err.Error()
	} else {
		st.SongCount = len(cata.Songs)
	}
	if _, err := os.Stat(filepath.Join(st.DataDir, "arcaea-backgrounds.json")); err != nil {
		if st.DataError != "" {
			st.DataError += "；"
		}
		st.DataError += "缺少背景清单：" + err.Error()
	}
	overrides, err := loadToolPaths()
	if err != nil {
		st.ToolError = err.Error()
	}
	st.Tools = tools.Detect(overrides).Statuses
	return st
}

type ToolCheck struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Source string `json:"source"`
	Result string `json:"result"`
}

// toolPathFile 永久记录需要的工具路径
func toolPathsFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rhythm-game-bvideo", "tool-paths.json"), nil
}

// loadToolPaths 通过文件记录的工具路径加载工具
func loadToolPaths() (tools.Tools, error) {
	var paths tools.Tools
	file, err := toolPathsFile()
	if err != nil {
		return paths, err
	}
	b, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return paths, nil
	}
	if err != nil {
		return paths, err
	}
	if err := json.Unmarshal(b, &paths); err != nil {
		return paths, fmt.Errorf("工具路径设置损坏：%w", err)
	}
	return paths, nil
}

// saveToolPaths 是保存工具路径用的函数
func saveToolPaths(paths tools.Tools) error {
	file, err := toolPathsFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), "tool-paths-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), file)
}

func toolSlot(paths *tools.Tools, name string) (*string, error) {
	switch name {
	case "ffmpeg":
		return &paths.FFmpeg, nil
	case "ffprobe":
		return &paths.FFprobe, nil
	case "edge":
		return &paths.Edge, nil
	case "yt-dlp":
		return &paths.YtDlp, nil
	default:
		return nil, fmt.Errorf("未知工具：%s", name)
	}
}

// SetToolPath 在验证通过后保存手动路径；空路径表示恢复自动检测
func (a *App) SetToolPath(name, path string) (ToolCheck, error) {
	var result ToolCheck
	paths, err := loadToolPaths()
	if err != nil {
		return result, err
	}
	slot, err := toolSlot(&paths, name)
	if err != nil {
		return result, err
	}
	if path == "" {
		*slot = ""
		for _, st := range tools.Detect(paths).Statuses {
			if st.Name != name {
				continue
			}
			if st.Path == "" {
				return result, fmt.Errorf("%s 自动检测未找到", name)
			}
			version, err := tools.Check(name, st.Path)
			if err != nil {
				return result, fmt.Errorf("%s 自动检测失败：%w", name, err)
			}
			if err := saveToolPaths(paths); err != nil {
				return result, err
			}
			return ToolCheck{Name: name, Path: st.Path, Source: st.Source, Result: version}, nil
		}
		return result, fmt.Errorf("未知工具：%s", name)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return result, err
	}
	result.Result, err = tools.Check(name, path)
	if err != nil {
		return result, err
	}
	*slot = path
	if err := saveToolPaths(paths); err != nil {
		return ToolCheck{}, err
	}
	return ToolCheck{Name: name, Path: path, Source: "override", Result: result.Result}, nil
}

// CheckTool 是检测工具的整个流程
func (a *App) CheckTool(name string) (ToolCheck, error) {
	var result ToolCheck
	paths, err := loadToolPaths()
	if err != nil {
		return result, err
	}
	if _, err := toolSlot(&paths, name); err != nil {
		return result, err
	}
	for _, st := range tools.Detect(paths).Statuses {
		if st.Name != name {
			continue
		}
		if st.Path == "" {
			return result, fmt.Errorf("%s 未找到，请手动选择", name)
		}
		version, err := tools.Check(name, st.Path)
		if err != nil {
			return result, err
		}
		return ToolCheck{Name: name, Path: st.Path, Source: st.Source, Result: version}, nil
	}
	return result, fmt.Errorf("未知工具：%s", name)
}
