package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Manager persists balance history and calculates daily averages.
type Manager struct {
	days int
}

// DailySpend captures spend per day.
type DailySpend struct {
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
}

// Record describes history file contents.
type Record struct {
	LastBalance float64      `json:"last_balance"`
	LastUpdated string       `json:"last_updated"`
	DailySpends []DailySpend `json:"daily_spends"`
}

// Result collects fresh spend and average information.
type Result struct {
	Spend   float64
	Average float64
}

// NewManager builds history manager for configured window.
func NewManager(days int) *Manager {
	if days < 1 {
		days = 1
	}
	return &Manager{days: days}
}

// Update consumes fresh balance, refreshes history, and returns spend stats.
func (m *Manager) Update(path string, balance float64, now time.Time) (Result, error) {
	history, err := m.load(path)
	if err != nil {
		return Result{}, err
	}

	spend := computeSpend(history.LastBalance, balance)
	dayKey := now.Format("2006-01-02")

	if len(history.DailySpends) == 0 || history.DailySpends[len(history.DailySpends)-1].Date != dayKey {
		history.DailySpends = append(history.DailySpends, DailySpend{Date: dayKey, Amount: spend})
	} else {
		history.DailySpends[len(history.DailySpends)-1].Amount = spend
	}

	if len(history.DailySpends) > m.days {
		history.DailySpends = history.DailySpends[len(history.DailySpends)-m.days:]
	}

	history.LastBalance = balance
	history.LastUpdated = now.Format(time.RFC3339)

	if err := m.save(path, history); err != nil {
		return Result{}, err
	}

	return Result{Spend: spend, Average: average(history.DailySpends)}, nil
}

func (m *Manager) load(path string) (Record, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Record{}, nil
		}
		return Record{}, fmt.Errorf("open history %q: %v", path, err)
	}
	defer file.Close()

	var history Record
	if err := json.NewDecoder(file).Decode(&history); err != nil {
		return Record{}, fmt.Errorf("decode history %q: %v", path, err)
	}
	return history, nil
}

func (m *Manager) save(path string, record Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create history dir: %v", err)
	}

	tmp := path + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create history tmp %q: %v", tmp, err)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(&record); err != nil {
		file.Close()
		return fmt.Errorf("encode history %q: %v", path, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close history %q: %v", path, err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace history %q: %v", path, err)
	}
	return nil
}

// computeSpend returns positive spend when balance decreases.
func computeSpend(previous, current float64) float64 {
	diff := previous - current
	if diff < 0 {
		return 0
	}
	return diff
}

func average(items []DailySpend) float64 {
	if len(items) == 0 {
		return 0
	}
	var total float64
	for _, item := range items {
		total += item.Amount
	}
	return total / float64(len(items))
}
