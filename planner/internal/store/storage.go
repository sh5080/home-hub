package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Storage는 "얼마나 더 넣을 수 있나"에 답하는 수치다.
//
// 플래너 DB 하나만 바로 그리면 의미가 없다 — 카드 수십 장은 SD카드의
// 0.001% 다. 실제 제약은 디스크이므로 디스크를 기준으로 삼고, 플래너가
// 그중 얼마를 쓰는지와 환산치를 함께 준다.
type Storage struct {
	// Quota는 플래너가 써도 되는 상한이다. 디스크 전체를 기준으로 삼으면
	// 바가 늘 0%라 아무 정보가 없고, 한도가 없으면 플래너가 SD카드를 채워
	// 홈허브까지 멈출 수 있다.
	Quota     int64  `json:"quota"`
	UsedBytes int64  `json:"used_bytes"` // db + 백업
	DiskTotal uint64 `json:"disk_total"`
	DiskFree  uint64 `json:"disk_free"`
	DiskUsed  uint64 `json:"disk_used"`

	DBBytes      int64 `json:"db_bytes"`     // planner.db (+ WAL)
	BackupBytes  int64 `json:"backup_bytes"` // backup-*.db 합계
	BackupCount  int   `json:"backup_count"`
	BytesPerCard int64 `json:"bytes_per_card"` // 환산 기준. 카드가 없으면 0

	Cards    int `json:"cards"`
	Boards   int `json:"boards"`
	Routines int `json:"routines"`
	Users    int `json:"users"`
}

// DefaultQuota는 플래너 기본 상한이다. 카드 한 장이 대략 2~3KB이므로
// 수십만 장에 해당한다 — 가족이 닿을 일이 없으면서, SD카드를 지킨다.
const DefaultQuota int64 = 512 << 20 // 512 MiB

// SetQuota overrides the limit (운영에서 --quota 로 준다).
func (s *Store) SetQuota(n int64) {
	if n > 0 {
		s.quota = n
	}
}

// QuotaExceeded는 한도를 넘었는지 본다. 새로 만드는 것만 막고 수정·삭제는
// 항상 허용한다 — 삭제까지 막으면 되돌릴 방법이 없어진다.
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
	// Bsize의 타입이 리눅스(int64)와 macOS(uint32)에서 달라 캐스팅한다.
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
		// 테이블 이름은 이 맵의 상수뿐이다 — 외부 입력이 닿지 않는다.
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM `+tbl).Scan(dst); err != nil {
			return out, err
		}
	}
	out.UsedBytes = out.DBBytes + out.BackupBytes
	if out.Cards > 0 {
		out.BytesPerCard = out.DBBytes / int64(out.Cards)
	}
	return out, nil
}
