package recordstore

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"doc-publish-server/internal/configloader"
)

type Service struct {
	dir     string
	counter uint64
}

func New(dir string) *Service {
	return &Service{dir: dir}
}

func (s *Service) Save(record *configloader.PublishRecord) error {
	if record.RecordID == "" {
		record.RecordID = s.NewRecordID()
	}
	return configloader.SaveYAML(filepath.Join(s.dir, "record_"+record.RecordID+".yaml"), record)
}

func (s *Service) List() ([]configloader.PublishRecord, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	records := make([]configloader.PublishRecord, 0, len(entries))
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		record := configloader.PublishRecord{}
		if err := loadRecord(filepath.Join(s.dir, entry.Name()), &record); err == nil {
			record.RecordID = strings.TrimSpace(record.RecordID)
			record.PublishingTime = strings.TrimSpace(record.PublishingTime)
			record.PublishingType = strings.TrimSpace(record.PublishingType)
			record.Status = strings.TrimSpace(record.Status)
			if record.RecordID == "" {
				continue
			}
			if _, exists := seen[record.RecordID]; exists {
				continue
			}
			seen[record.RecordID] = struct{}{}
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].PublishingTime == records[j].PublishingTime {
			return records[i].RecordID > records[j].RecordID
		}
		return records[i].PublishingTime > records[j].PublishingTime
	})
	return records, nil
}

func (s *Service) Detail(recordID string) (*configloader.PublishRecord, error) {
	record := &configloader.PublishRecord{}
	if err := loadRecord(filepath.Join(s.dir, "record_"+recordID+".yaml"), record); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *Service) NewRecordID() string {
	seq := atomic.AddUint64(&s.counter, 1)
	return time.Now().Format("20060102_1504") + "_" + pad3(seq)
}

func loadRecord(path string, record *configloader.PublishRecord) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yamlUnmarshal(raw, record)
}

func pad3(v uint64) string {
	if v < 10 {
		return "00" + strconvFormat(v)
	}
	if v < 100 {
		return "0" + strconvFormat(v)
	}
	return strconvFormat(v)
}
