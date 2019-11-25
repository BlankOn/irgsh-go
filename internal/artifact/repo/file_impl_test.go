package repo

import (
	"os"
	"reflect"
	"testing"
)

func TestMain(m *testing.M) {
	// prepare artifact file
	os.Mkdir("./artifacts", os.ModePerm)
	file001, _ := os.Create("./artifacts/file001")
	file001.Close()
	file002, _ := os.Create("./artifacts/file002")
	file002.Close()

	exitVal := m.Run()
	// time.Sleep(2 * time.Second)

	// clean up test directory
	os.RemoveAll("./artifacts")

	os.Exit(exitVal)
}

func Test_getArtifactFilename(t *testing.T) {
	type args struct {
		filePath string
	}
	tests := []struct {
		name         string
		args         args
		wantFileName string
	}{
		{
			name: "empty",
		},
		{
			name: "correct : /var/www/artifacts/xxxyyyzzz",
			args: args{
				filePath: "/var/www/artifacts/xxxyyyzzz",
			},
			wantFileName: "xxxyyyzzz",
		},
		{
			name: "error : /var/www/xxxyyyzzz",
			args: args{
				filePath: "/var/www/xxxyyyzzz",
			},
			wantFileName: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotFileName := getArtifactFilename(tt.args.filePath); gotFileName != tt.wantFileName {
				t.Errorf("getArtifactFilename() = %v, want %v", gotFileName, tt.wantFileName)
			}
		})
	}
}

func TestFileRepo_GetArtifactList(t *testing.T) {
	type fields struct {
		Workdir string
	}
	type args struct {
		pageNum int64
		rows    int64
	}
	tests := []struct {
		name              string
		fields            fields
		args              args
		wantArtifactsList ArtifactList
		wantErr           bool
	}{
		{
			name: "get files",
			fields: fields{
				Workdir: ".",
			},
			wantArtifactsList: ArtifactList{
				TotalData: 2,
				Artifacts: []ArtifactModel{
					ArtifactModel{Name: "file001"},
					ArtifactModel{Name: "file002"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			A := &FileRepo{
				Workdir: tt.fields.Workdir,
			}
			gotArtifactsList, err := A.GetArtifactList(tt.args.pageNum, tt.args.rows)
			if (err != nil) != tt.wantErr {
				t.Errorf("FileRepo.GetArtifactList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotArtifactsList, tt.wantArtifactsList) {
				t.Errorf("FileRepo.GetArtifactList() = %v, want %v", gotArtifactsList, tt.wantArtifactsList)
			}
		})
	}
}
