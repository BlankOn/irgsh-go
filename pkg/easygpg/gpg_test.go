package easypgp

import "testing"

func TestEasyPGP_Verify(t *testing.T) {
	type fields struct {
		gpgCmd string
	}
	type args struct {
		dirPath  string
		fileName string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantOk  bool
		wantErr bool
	}{
		{
			name:   "piko",
			args:   args{dirPath: "/home/gio/Downloads/", fileName: "piko.dsc"},
			wantOk: true,
		},
		{
			name:   "wildcard",
			args:   args{dirPath: "/home/gio/Downloads/", fileName: "*.dsc"},
			wantOk: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			E := EasyPGP{}
			gotOk, err := E.Verify(tt.args.dirPath, tt.args.fileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("EasyPGP.Verify() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotOk != tt.wantOk {
				t.Errorf("EasyPGP.Verify() = %v, want %v", gotOk, tt.wantOk)
			}
		})
	}
}
