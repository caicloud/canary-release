package chart

import "testing"

func TestReplaceConfig(t *testing.T) {
	type args struct {
		origin   string
		path     string
		newValue string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			"",
			args{
				`{"_config":{},"test":{}}`,
				"path",
				`{"_config":{"_metadata":{}}}`,
			},
			`{"_config":{"_metadata":{"revision":2}},"test":{}}`,
			false,
		},
		{
			"",
			args{
				`{"_config":{"_metadata":{"revision":3}},"test":{}}`,
				"path",
				`{"_config":{"_metadata":{}}}`,
			},
			`{"_config":{"_metadata":{"revision":4}},"test":{}}`,
			false,
		},
		{
			"",
			args{
				`{"_config":{},"test":{"_config":{},"test2":{}}}`,
				"path/test",
				`{"_config":{"_metadata":{}}}`,
			},
			`{"_config":{},"test":{"_config":{"_metadata":{"revision":2}},"test2":{}}}`,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReplaceConfig(tt.args.origin, tt.args.path, tt.args.newValue)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReplaceConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ReplaceConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}
