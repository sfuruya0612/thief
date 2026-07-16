package cli

import "testing"

func TestSplitBqTableRef(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantDataset string
		wantTable   string
		wantErr     bool
	}{
		{name: "valid", input: "mydataset.mytable", wantDataset: "mydataset", wantTable: "mytable"},
		{name: "table name with dots", input: "ds.table.v1", wantDataset: "ds", wantTable: "table.v1"},
		{name: "missing dot", input: "mytable", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataset, table, err := splitBqTableRef(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("splitBqTableRef(%q) err = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if dataset != tt.wantDataset || table != tt.wantTable {
				t.Errorf("splitBqTableRef(%q) = (%q, %q), want (%q, %q)",
					tt.input, dataset, table, tt.wantDataset, tt.wantTable)
			}
		})
	}
}
