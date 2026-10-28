// Package notegen integrates local Codex with amux note generation.
package notegen

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Model struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	DisplayName string `json:"displayName"`
	IsDefault   bool   `json:"isDefault"`
	Hidden      bool   `json:"hidden"`
}

// Models asks the installed Codex app-server for its available model catalog.
func Models(ctx context.Context) ([]Model, error) {
	cmd := exec.CommandContext(ctx, "codex", "app-server")
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		in.Close()
		return nil, fmt.Errorf("start Codex: %w", err)
	}
	defer func() {
		in.Close()
		cmd.Process.Kill()
		cmd.Wait()
	}()
	enc := json.NewEncoder(in)
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	call := func(id int, method string, params any, result any) error {
		if err := enc.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
			return err
		}
		for scanner.Scan() {
			var msg struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				return fmt.Errorf("Codex response: %w", err)
			}
			if msg.ID != id {
				continue
			}
			if msg.Error != nil {
				return fmt.Errorf("Codex %s: %s", method, msg.Error.Message)
			}
			return json.Unmarshal(msg.Result, result)
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return fmt.Errorf("Codex closed before %s response", method)
	}
	var initialized json.RawMessage
	if err := call(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "amux", "version": "1"}}, &initialized); err != nil {
		return nil, err
	}
	if err := enc.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return nil, err
	}
	var models []Model
	var cursor *string
	for id := 2; ; id++ {
		var page struct {
			Data       []Model `json:"data"`
			NextCursor *string `json:"nextCursor"`
		}
		if err := call(id, "model/list", map[string]any{"cursor": cursor}, &page); err != nil {
			return nil, err
		}
		for _, model := range page.Data {
			if !model.Hidden && model.Model != "" {
				models = append(models, model)
			}
		}
		if page.NextCursor == nil {
			break
		}
		if cursor != nil && *cursor == *page.NextCursor {
			return nil, fmt.Errorf("Codex repeated model cursor")
		}
		cursor = page.NextCursor
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("Codex returned no available models")
	}
	return models, nil
}

func modelPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "amux", "note-model.json"), nil
}

// SelectedModel is independent of layout preferences, so refresh cannot reset it.
// Empty means use Codex's default.
func SelectedModel() string {
	path, err := modelPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var model string
	if json.Unmarshal(data, &model) != nil {
		return ""
	}
	return model
}

func SelectModel(model string) error {
	path, err := modelPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".note-model-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(model)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}
