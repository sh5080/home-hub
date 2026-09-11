package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// FinWallet 은 충전식 지갑(지역화폐 등) 하나.
type FinWallet struct {
	ID            int64            `json:"id"`
	OwnerID       int64            `json:"owner_id"`
	Name          string           `json:"name"`
	Match         string           `json:"match"`
	BaseBalance   int64            `json:"base_balance"`   // 맞출 때 합계(충전 + 인센티브)
	BaseIncentive int64            `json:"base_incentive"` // 그중 인센티브
	BaseAt        string           `json:"base_at"`
	Rate          float64          `json:"rate"`    // 충전 인센티브(0.1 = 10%)
	Balance       int64            `json:"balance"` // 지금 합계
	Charged       int64            `json:"charged"` // 이번 달 충전(통장에서 나간 돈)
	Earned        int64            `json:"earned"`  // 이번 달 적립 인센티브
	Spent         int64            `json:"spent"`   // 이번 달 사용
	Recent        []FinWalletSpend `json:"recent"`  // 최근 사용(최대 30)
	Charges       []FinWalletMove  `json:"charges"` // 최근 충전(최대 10)
}

type FinWalletSpend struct {
	ID     int64  `json:"id"`
	At     string `json:"at"`
	Title  string `json:"title"`
	Amount int64  `json:"amount"`
}

type FinWalletMove struct {
	At     string `json:"at"`
	Amount int64  `json:"amount"`
}

func (s *Store) finWalletRows(ctx context.Context, owner int64) ([]FinWallet, error) {
	q := `SELECT id, owner_id, name, match, base_balance, base_incentive, base_at, incentive_rate FROM fin_wallets`
	var args []any
	if owner != 0 {
		q += ` WHERE owner_id = ?`
		args = append(args, owner)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FinWallet
	for rows.Next() {
		var w FinWallet
		if err := rows.Scan(&w.ID, &w.OwnerID, &w.Name, &w.Match, &w.BaseBalance, &w.BaseIncentive, &w.BaseAt, &w.Rate); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// FinanceWallets 는 지갑들과 잔액. now 의 달을 '이번 달'로 센다.
func (s *Store) FinanceWallets(ctx context.Context, owner int64, now time.Time) ([]FinWallet, error) {
	ws, err := s.finWalletRows(ctx, owner)
	if err != nil {
		return nil, err
	}
	month := now.Format("2006-01")
	for i := range ws {
		w := &ws[i]
		w.Recent, w.Charges = []FinWalletSpend{}, []FinWalletMove{}
		// 충전: 통장 내역에서 나간 이체 중 이름이 든 것. 기준 시점 이후만 잔액에 더한다.
		crows, err := s.db.QueryContext(ctx,
			`SELECT at, -amount FROM fin_tx WHERE owner_id=? AND type='이체' AND amount < 0 AND instr(content, ?) > 0 ORDER BY at DESC`, w.OwnerID, w.Match)
		if err != nil {
			return nil, err
		}
		var charges []FinWalletMove
		for crows.Next() {
			var m FinWalletMove
			if err := crows.Scan(&m.At, &m.Amount); err != nil {
				crows.Close()
				return nil, err
			}
			charges = append(charges, m)
		}
		crows.Close()
		// 충전하면 인센티브가 같이 들어온다. 쓴 돈은 적은 금액 그대로 뺀다.
		w.Balance = w.BaseBalance
		for _, m := range charges {
			bonus := int64(float64(m.Amount)*w.Rate + 0.5)
			if m.At > w.BaseAt {
				w.Balance += m.Amount + bonus
			}
			if m.At[:7] == month {
				w.Charged += m.Amount
				w.Earned += bonus
			}
			if len(w.Charges) < 10 {
				w.Charges = append(w.Charges, m)
			}
		}
		srows, err := s.db.QueryContext(ctx, `SELECT id, at, title, amount FROM fin_wallet_spends WHERE wallet_id=? ORDER BY at DESC, id DESC`, w.ID)
		if err != nil {
			return nil, err
		}
		for srows.Next() {
			var x FinWalletSpend
			if err := srows.Scan(&x.ID, &x.At, &x.Title, &x.Amount); err != nil {
				srows.Close()
				return nil, err
			}
			if x.At > w.BaseAt {
				w.Balance -= x.Amount
			}
			if x.At[:7] == month {
				w.Spent += x.Amount
			}
			if len(w.Recent) < 30 {
				w.Recent = append(w.Recent, x)
			}
		}
		srows.Close()
	}
	if ws == nil {
		ws = []FinWallet{}
	}
	return ws, nil
}

// finWalletSpendRows 는 overview 에 쓴 돈으로 넣을 사용 기록(from 이후).
func (s *Store) finWalletSpendRows(ctx context.Context, owner int64, from string) ([]finRow, error) {
	q := `SELECT x.id, w.owner_id, w.name, x.at, x.title, x.amount FROM fin_wallet_spends x JOIN fin_wallets w ON w.id = x.wallet_id WHERE x.at >= ?`
	args := []any{from}
	if owner != 0 {
		q += ` AND w.owner_id = ?`
		args = append(args, owner)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []finRow
	for rows.Next() {
		var r finRow
		var name string
		var amt, id int64
		if err := rows.Scan(&id, &r.owner, &name, &r.At, &r.Content, &amt); err != nil {
			return nil, err
		}
		r.Type, r.Cat1, r.Method, r.Amount, r.wallet = "지출", name, name, -amt, true
		r.ref = "w" + strconv.FormatInt(id, 10)
		out = append(out, r)
	}
	return out, rows.Err()
}

// 이름은 따로 정하지 않는다 — 비면 '지역화폐'이고, 충전은 이름이 든 이체로 찾는다('경기지역화폐' 등).
func finWalletInput(name, match string) (string, string, error) {
	name, match = strings.TrimSpace(name), strings.TrimSpace(match)
	if name == "" {
		name = "지역화폐"
	}
	if len([]rune(name)) > 30 {
		return "", "", invalid("이름은 30자까지예요")
	}
	if match == "" {
		match = name
	}
	return name, match, nil
}

// FinanceSetWalletBalance 는 그 사람의 지역화폐 잔액(충전 잔액 + 인센티브 잔액)을 지금 기준으로 맞춘다.
// 없으면 만든다(사람마다 하나). 지금까지 쓴 건 이미 빠진 값이다 — 이전 사용 기록은 잔액에 다시 반영하지 않는다.
func (s *Store) FinanceSetWalletBalance(ctx context.Context, owner, cash, incentive int64, now time.Time) (int64, error) {
	if cash < 0 || incentive < 0 {
		return 0, invalid("잔액은 0 이상이에요")
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM fin_wallets WHERE owner_id=? ORDER BY id LIMIT 1`, owner).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		if id, err = s.FinanceCreateWallet(ctx, owner, "", "", cash+incentive, now); err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE fin_wallets SET base_balance=?, base_incentive=?, base_at=? WHERE id=?`,
		cash+incentive, incentive, now.Format("2006-01-02T15:04"), id)
	return id, err
}

func (s *Store) FinanceCreateWallet(ctx context.Context, owner int64, name, match string, balance int64, now time.Time) (int64, error) {
	name, match, err := finWalletInput(name, match)
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO fin_wallets (owner_id, name, match, base_balance, base_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		owner, name, match, balance, now.Format("2006-01-02T15:04"), now.Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinanceDeleteWallet(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM fin_wallets WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func finSpendInput(at, title string, amount int64, now time.Time) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > 60 {
		return "", invalid("제목은 1~60자예요")
	}
	if amount <= 0 || amount > 100_000_000 {
		return "", invalid("금액을 확인해주세요")
	}
	t, err := time.ParseInLocation("2006-01-02T15:04", at, now.Location())
	if err != nil {
		return "", invalid("날짜·시각이 올바르지 않아요")
	}
	if t.After(now.Add(5 * time.Minute)) {
		return "", invalid("아직 오지 않은 시각이에요")
	}
	return title, nil
}

func (s *Store) FinanceUpdateWalletSpend(ctx context.Context, id int64, at, title string, amount int64, now time.Time) error {
	title, err := finSpendInput(at, title, amount, now)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE fin_wallet_spends SET at=?, title=?, amount=? WHERE id=?`, at, title, amount, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) FinanceAddWalletSpend(ctx context.Context, walletID int64, at, title string, amount int64, by int64, now time.Time) (int64, error) {
	title, err := finSpendInput(at, title, amount, now)
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO fin_wallet_spends (wallet_id, at, title, amount, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		walletID, at, title, amount, by, now.Unix())
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinanceDeleteWalletSpend(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM fin_tx_tags WHERE ref=?`, "w"+strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM fin_wallet_spends WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
