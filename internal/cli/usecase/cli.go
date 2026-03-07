package usecase

// CLIUsecase orchestrates all CLI business logic through port interfaces.
type CLIUsecase struct {
	config    ConfigStore
	pipelines PipelineStore
	chief     ChiefAPI
	shell     ShellRunner
	repoSync  RepoSync
	debian    DebianPackager
	gpg       GPGSigner
	releases  ReleaseFetcher
	updater   UpdateApplier
	prompter  Prompter
	version   string
}

func NewCLIUsecase(
	config ConfigStore,
	pipelines PipelineStore,
	chief ChiefAPI,
	shell ShellRunner,
	repoSync RepoSync,
	debian DebianPackager,
	gpg GPGSigner,
	releases ReleaseFetcher,
	updater UpdateApplier,
	prompter Prompter,
	version string,
) *CLIUsecase {
	return &CLIUsecase{
		config:    config,
		pipelines: pipelines,
		chief:     chief,
		shell:     shell,
		repoSync:  repoSync,
		debian:    debian,
		gpg:       gpg,
		releases:  releases,
		updater:   updater,
		prompter:  prompter,
		version:   version,
	}
}
