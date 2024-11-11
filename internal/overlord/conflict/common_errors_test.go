package conflict_test

var (
	noRefresh           = ".* no refresh in progress"
	resumeDuringLaunch  = `.* no refresh in progress \("launch" is in progress\)`
	resumeDuringRefresh = ".* no refresh is waiting on error"
)
