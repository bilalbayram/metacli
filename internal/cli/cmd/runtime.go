package cmd

type Runtime struct {
	Profile *string
	Output  *string
	Debug   *bool
}

func (r Runtime) ProfileName() string {
	if r.Profile == nil {
		return ""
	}
	return *r.Profile
}
