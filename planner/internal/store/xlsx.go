package store

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
)

// 엑셀(xlsx) 최소 읽기: 시트별로 줄마다 {열 문자: 값}. 수식 칸은 저장된 값만 읽는다.
type xlsxRow map[string]string

func readXLSX(data []byte) (map[string][]xlsxRow, []string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, invalid("엑셀 파일이 아니에요")
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	read := func(name string) ([]byte, error) {
		f := files[name]
		if f == nil {
			return nil, fmt.Errorf("xlsx: %s 없음", name)
		}
		if f.UncompressedSize64 > 64<<20 {
			return nil, invalid("엑셀이 너무 커요")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}

	var shared []string
	if b, err := read("xl/sharedStrings.xml"); err == nil {
		var sst struct {
			SI []struct {
				T string `xml:"t"`
				R []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"si"`
		}
		if err := xml.Unmarshal(b, &sst); err != nil {
			return nil, nil, err
		}
		for _, si := range sst.SI {
			s := si.T
			for _, r := range si.R {
				s += r.T
			}
			shared = append(shared, s)
		}
	}

	wbb, err := read("xl/workbook.xml")
	if err != nil {
		return nil, nil, invalid("엑셀 파일이 아니에요")
	}
	var wb struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := xml.Unmarshal(wbb, &wb); err != nil {
		return nil, nil, err
	}
	relb, err := read("xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, nil, err
	}
	var rels struct {
		R []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(relb, &rels); err != nil {
		return nil, nil, err
	}
	target := map[string]string{}
	for _, r := range rels.R {
		t := strings.TrimPrefix(r.Target, "/")
		if !strings.HasPrefix(t, "xl/") {
			t = path.Join("xl", t)
		}
		target[r.ID] = t
	}

	out := map[string][]xlsxRow{}
	var order []string
	for _, sh := range wb.Sheets {
		b, err := read(target[sh.RID])
		if err != nil {
			return nil, nil, err
		}
		var ws struct {
			Rows []struct {
				C []struct {
					R  string `xml:"r,attr"`
					T  string `xml:"t,attr"`
					V  string `xml:"v"`
					IS struct {
						T string `xml:"t"`
					} `xml:"is"`
				} `xml:"c"`
			} `xml:"sheetData>row"`
		}
		if err := xml.Unmarshal(b, &ws); err != nil {
			return nil, nil, err
		}
		rows := make([]xlsxRow, 0, len(ws.Rows))
		for _, r := range ws.Rows {
			row := xlsxRow{}
			for _, c := range r.C {
				col := strings.TrimRight(c.R, "0123456789")
				v := c.V
				switch c.T {
				case "s":
					var i int
					if _, err := fmt.Sscanf(v, "%d", &i); err == nil && i >= 0 && i < len(shared) {
						v = shared[i]
					}
				case "inlineStr":
					v = c.IS.T
				}
				if v != "" {
					row[col] = v
				}
			}
			rows = append(rows, row)
		}
		out[sh.Name] = rows
		order = append(order, sh.Name)
	}
	return out, order, nil
}
