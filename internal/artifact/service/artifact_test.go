package service

import (
	"fmt"
	"reflect"
	"testing"

	artifactRepo "github.com/blankon/irgsh-go/internal/artifact/repo"
)

// func TestArtifactService_GetArtifactList(t *testing.T) {
// 	type fields struct {
// 		repo artifactRepo.Repo
// 	}
// 	type args struct {
// 		pageNum int64
// 		rows    int64
// 	}
// 	tests := []struct {
// 		name      string
// 		fields    fields
// 		args      args
// 		wantItems []ArtifactItem
// 		wantErr   bool
// 	}{
// 		{
// 			name: "empty",
// 			fields: fields{
// 				repo: &artifactRepo.RepoMock{
// 					GetArtifactListFunc: func(pageNum int64, rows int64) (m []repo.ArtifactModel, e error) {
// 						return
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "error",
// 			fields: fields{
// 				repo: &artifactRepo.RepoMock{
// 					GetArtifactListFunc: func(pageNum int64, rows int64) (m []repo.ArtifactModel, e error) {
// 						return m, fmt.Errorf("")
// 					},
// 				},
// 			},
// 			wantErr: true,
// 		},
// 		{
// 			name: "2 items",
// 			fields: fields{
// 				repo: &artifactRepo.RepoMock{
// 					GetArtifactListFunc: func(pageNum int64, rows int64) (m []repo.ArtifactModel, e error) {
// 						m = append(m, repo.ArtifactModel{Name: "test1"})
// 						m = append(m, repo.ArtifactModel{Name: "test2"})
// 						return
// 					},
// 				},
// 			},
// 			wantItems: []ArtifactItem{ArtifactItem{Name: "test1"}, ArtifactItem{Name: "test2"}},
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			A := &ArtifactService{
// 				repo: tt.fields.repo,
// 			}
// 			gotItems, err := A.GetArtifactList(tt.args.pageNum, tt.args.rows)
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("ArtifactService.GetArtifactList() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if !reflect.DeepEqual(gotItems, tt.wantItems) {
// 				t.Errorf("ArtifactService.GetArtifactList() = %v, want %v", gotItems, tt.wantItems)
// 			}
// 		})
// 	}
// }

func TestNewArtifactService(t *testing.T) {
	type args struct {
		repo artifactRepo.Repo
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
			if got := NewArtifactService(tt.args.repo); !reflect.DeepEqual(got, tt.want) {
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
			wantArtifacts: ArtifactList{TotalData: 0, Artifacts: []ArtifactItem{}},
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
						l.Artifacts = append(l.Artifacts, artifactRepo.ArtifactModel{Name: "test1"})
						l.Artifacts = append(l.Artifacts, artifactRepo.ArtifactModel{Name: "test2"})
						l.TotalData = 2
						return
					},
				},
			},
			wantArtifacts: ArtifactList{
				TotalData: 2,
				Artifacts: []ArtifactItem{ArtifactItem{Name: "test1"}, ArtifactItem{Name: "test2"}},
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
