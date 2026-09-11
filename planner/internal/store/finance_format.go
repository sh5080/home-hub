package store

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// 재정 파일 규격. 어떤 서비스의 내보내기를 읽을지는 코드가 아니라 이 설정이 정한다
// (서비스 이름과 시트·절·열 이름을 저장소에 두지 않는다). 규격 설명: docs/finance-format.md
type FinFormat struct {
	SourceName string `json:"source_name"` // 화면에 보일 서비스 이름
	HowTo      string `json:"how_to"`      // 파일 받는 법(화면 안내)

	SummarySheet string `json:"summary_sheet"` // 자산·부채 요약 시트
	TxSheet      string `json:"tx_sheet"`      // 거래 시트

	// 요약 시트 안 절 제목(B열 'n.제목'의 제목 부분)
	BalanceSection string `json:"balance_section"`
	InvestSection  string `json:"invest_section"`
	LoanSection    string `json:"loan_section"`

	// 자산 분류 이름 → 묶음(liquid|saving|invest|insurance|pension|other). 없는 분류는 other.
	AssetGroups map[string]string `json:"asset_groups"`
	// 자산·부채 표가 끝나는 줄(B열이 이 말로 시작)
	BalanceEnd string `json:"balance_end"`

	// 거래 시트 머리줄. 열 문자 → 기대하는 머리말. 파일이 다르면 어느 열이 다른지 알려준다.
	TxHeader map[string]string `json:"tx_header"`
	// 거래 유형 값
	TypeIn       string `json:"type_in"`
	TypeOut      string `json:"type_out"`
	TypeTransfer string `json:"type_transfer"`
}

// txCol 은 머리말로 열 문자를 찾는다.
func (f *FinFormat) txCol(header string) string {
	for col, h := range f.TxHeader {
		if h == header {
			return col
		}
	}
	return ""
}

var finTxFields = []string{"날짜", "시간", "타입", "대분류", "소분류", "내용", "금액", "결제수단", "메모"}

func (f *FinFormat) validate() error {
	var miss []string
	for k, v := range map[string]string{
		"summary_sheet": f.SummarySheet, "tx_sheet": f.TxSheet, "balance_section": f.BalanceSection,
		"balance_end": f.BalanceEnd, "type_in": f.TypeIn, "type_out": f.TypeOut, "type_transfer": f.TypeTransfer,
	} {
		if strings.TrimSpace(v) == "" {
			miss = append(miss, k)
		}
	}
	for _, h := range finTxFields {
		if f.txCol(h) == "" {
			miss = append(miss, "tx_header."+h)
		}
	}
	if len(miss) > 0 {
		return fmt.Errorf("자산 파일 규격 설정에 빠진 값: %s", strings.Join(miss, ", "))
	}
	return nil
}

// LoadFinFormat 은 규격 파일을 읽는다. 파일이 없으면 nil(가져오기 꺼짐).
func LoadFinFormat(path string) (*FinFormat, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f FinFormat
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := f.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}

// SetFinFormat / FinFormatNow: 서버가 시작할 때 한 번 넣는다.
func (s *Store) SetFinFormat(f *FinFormat) { s.finFormat = f }
func (s *Store) FinFormatNow() *FinFormat  { return s.finFormat }
