package main

type IrgshConfig struct {
	Redis   string        `json:"redis"`
	Chief   ChiefConfig   `json:"chief"`
	Builder BuilderConfig `json:"builder"`
	Repo    RepoConfig    `json:"repo"`
}

type ChiefConfig struct {
	Address string `json:"address" validate:"required"`
	Workdir string `json:"workdir" validate:"required"`
}

type BuilderConfig struct {
	Workdir string `json:"workdir" validate:"required"`
}

type RepoConfig struct {
	Workdir                    string `json:"workdir" validate:"required"`
	DistName                   string `json:"dist_name" validate:"required"`                    // BlankOn
	DistLabel                  string `json:"dist_label" validate:"required"`                   // BlankOn
	DistCodename               string `json:"dist_codename" validate:"required"`                // verbeek
	DistComponents             string `json:"dist_components" validate:"required"`              // main restricted extras extras-restricted
	DistSupportedArchitectures string `json:"dist_supported_architectures" validate:"required"` // amd64 source
	DistVersion                string `json:"dist_version" validate:"required"`                 // 12.0
	DistVersionDesc            string `json:"dist_version_desc" validate:"required"`            // BlankOn Linux 12.0 Verbeek
	DistSigningKey             string `json:"dist_signing_key" validate:"required"`             // 55BD65A0B3DA3A59ACA60932E2FE388D53B56A71
	UpstreamName               string `json:"upstream_name" validate:"required"`                // merge.sid
	UpstreamDistCodename       string `json:"upstream_dist_codename" validate:"required"`       // sid
	UpstreamDistUrl            string `json:"upstream_dist_url" validate:"required"`            // http://kartolo.sby.datautama.net.id/debian
	UpstreamDistComponents     string `json:"upstream_dist_components" validate:"required"`     // main non-free>restricted contrib>extras
}
