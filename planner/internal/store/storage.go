package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Storage 는 '얼마나 더 넣을 수 있나'에 답한다(디스크 기준 + 플래너 사용량).
type Storage struct {
	// Quota 는 플래너 상한. 없으면 SD카드를 채워 홈허브까지 멈출 수 있다.
	Quota      int64  `json:"quota"`
	UsedBytes  int64  `json:"used_bytes"` // db + 백업 + 사진
	MediaBytes int64  `json:"media_bytes"`
	MediaCount int    `json:"media_count"`
	DiskTotal  uint64 `json:"disk_total"`
	DiskFree   uint64 `json:"disk_free"`
	DiskUsed   uint64 `json:"disk_used"`

	DBBytes      int64 `json:"db_bytes"`     // planner.db (+ WAL)
	BackupBytes  int64 `json:"backup_bytes"` // backup-*.db 합계
	BackupCount  int   `json:"backup_count"`
	BytesPerCard int64 `json:"bytes_per_card"` // 환산 기준. 카드가 없으면 0

	Cards    int `json:"cards"`
	Boards   int `json:"boards"`
	Routines int `json:"routines"`
	Users    int `json:"users"`
}

const DefaultQuota int64 = 512 << 20 // 512 MiB

// SetQuota overrides the limit (운영에서 --quota 로 준다).
func (s *Store) SetQuota(n int64) {
	if n > 0 {
		s.quota = n
	}
}

// QuotaExceeded 는 한도 초과 여부. 새로 만들기만 막는다.
func (s *Store) QuotaExceeded(ctx context.Context) (bool, Storage, error) {
	st, err := s.Stat(ctx)
	if err != nil {
		return false, st, err
	}
	return st.UsedBytes >= st.Quota, st, nil
}

// Stat reads disk and database usage for the data directory.
func (s *Store) Stat(ctx context.Context) (Storage, error) {
	var out Storage
	out.Quota = s.quota
	if out.Quota == 0 {
		out.Quota = DefaultQuota
	}
	dir := filepath.Dir(s.path)

	var fs syscall.Statfs_t
	if err := syscall.Statfs(dir, &fs); err != nil {
		return out, err
	}
	// Bsize 타입이 리눅스(int64)/macOS(uint32)에서 다르다.
	block := uint64(fs.Bsize)
	out.DiskTotal = fs.Blocks * block
	out.DiskFree = fs.Bavail * block // 일반 사용자가 쓸 수 있는 양(예약분 제외)
	out.DiskUsed = out.DiskTotal - fs.Bfree*block

	entries, err := os.ReadDir(dir)
	if err != nil {
		return out, err
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := e.Name()
		switch {
		case strings.HasPrefix(name, "backup-") && strings.HasSuffix(name, ".db"):
			out.BackupBytes += info.Size()
			out.BackupCount++
		case strings.HasPrefix(name, "planner.db"): // -wal, -shm 포함
			out.DBBytes += info.Size()
		}
	}

	for tbl, dst := range map[string]*int{
		"cards": &out.Cards, "boards": &out.Boards, "routines": &out.Routines, "users": &out.Users,
	} {
		// 테이블 이름은 이 맵의 상수뿐.
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM `+tbl).Scan(dst); err != nil {
			return out, err
		}
	}
	// 사진은 파일이라 디렉터리 합계에 안 잡힌다 — 표에서 더한다.
	if n, c, err := s.MediaBytes(ctx); err == nil {
		out.MediaBytes, out.MediaCount = n, c
	}
	out.UsedBytes = out.DBBytes + out.BackupBytes + out.MediaBytes
	if out.Cards > 0 {
		out.BytesPerCard = out.DBBytes / int64(out.Cards)
	}
	return out, nil
}
