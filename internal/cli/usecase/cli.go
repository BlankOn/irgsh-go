package usecase

// CLIUsecase orchestrates all CLI business logic through port interfaces.
type CLIUsecase struct {
	Config    ConfigStore
	Pipelines PipelineStore
	Chief     ChiefAPI
	Shell     ShellRunner
	RepoSync  RepoSync
	Debian    DebianPackager
	GPG       GPGSigner
	Releases  ReleaseFetcher
	Updater   UpdateApplier
	Prompter  Prompter
	Version   string
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
		Config:    config,
		Pipelines: pipelines,
		Chief:     chief,
		Shell:     shell,
		RepoSync:  repoSync,
		Debian:    debian,
		GPG:       gpg,
		Releases:  releases,
		Updater:   updater,
		Prompter:  prompter,
		Version:   version,
	}
}
