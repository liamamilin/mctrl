package api

func desktopStartCommand(profile string) string {
	if profile == "" || profile == "v1" {
		return "mctrl here"
	}
	return "mctrl --profile " + profile + " here"
}
