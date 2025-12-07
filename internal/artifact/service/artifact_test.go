package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/RichardKnop/machinery/v1"
	artifactModel "github.com/blankon/irgsh-go/internal/artifact/model"
	artifactRepo "github.com/blankon/irgsh-go/internal/artifact/repo"
)

func TestNewArtifactService(t *testing.T) {
	type args struct {
		repo            artifactRepo.Repo
		machineryserver *machinery.Server
	}
	tests := []struct {
		name string
		args args
		want *ArtifactService
	}{
		{
			name: "empty",
			want: &ArtifactService{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewArtifactService(tt.args.repo, tt.args.machineryserver); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewArtifactService() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestArtifactService_GetArtifactList(t *testing.T) {
	type fields struct {
		repo artifactRepo.Repo
	}
	type args struct {
		pageNum int64
		rows    int64
	}
	tests := []struct {
		name          string
		fields        fields
		args          args
		wantArtifacts ArtifactList
		wantErr       bool
	}{
		{
			name: "empty",
			fields: fields{
				repo: &artifactRepo.RepoMock{
					GetArtifactListFunc: func(pageNum int64, rows int64) (l artifactRepo.ArtifactList, e error) {
						return
					},
				},
			},
			wantArtifacts: ArtifactList{TotalData: 0, Artifacts: []artifactModel.Artifact{}},
		},
		{
			name: "error",
			fields: fields{
				repo: &artifactRepo.RepoMock{
					GetArtifactListFunc: func(pageNum int64, rows int64) (l artifactRepo.ArtifactList, e error) {
						return l, fmt.Errorf("")
					},
				},
			},
			wantErr: true,
		},
		{
			name: "2 items",
			fields: fields{
				repo: &artifactRepo.RepoMock{
					GetArtifactListFunc: func(pageNum int64, rows int64) (l artifactRepo.ArtifactList, e error) {
						l.Artifacts = append(l.Artifacts, artifactModel.Artifact{Name: "test1"})
						l.Artifacts = append(l.Artifacts, artifactModel.Artifact{Name: "test2"})
						l.TotalData = 2
						return
					},
				},
			},
			wantArtifacts: ArtifactList{
				TotalData: 2,
				Artifacts: []artifactModel.Artifact{{Name: "test1"}, {Name: "test2"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			A := &ArtifactService{
				repo: tt.fields.repo,
			}
			gotArtifacts, err := A.GetArtifactList(tt.args.pageNum, tt.args.rows)
			if (err != nil) != tt.wantErr {
				t.Errorf("ArtifactService.GetArtifactList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotArtifacts, tt.wantArtifacts) {
				t.Errorf("ArtifactService.GetArtifactList() = %v, want %v", gotArtifacts, tt.wantArtifacts)
			}
		})
	}
}

func Test_generateSubmissionUUID(t *testing.T) {
	type args struct {
		timestamp time.Time
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "14 november 2020",
			args: args{
				timestamp: time.Date(2020, 11, 14, 5, 0, 0, 0, time.Now().Location()),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := generateSubmissionUUID(tt.args.timestamp); !strings.HasPrefix(got, "2020-11-14-050000_") {
				t.Errorf("generateSubmissionUUID() = %v, want %v", got, tt.want)
			}
		})
	}
}
