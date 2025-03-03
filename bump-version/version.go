package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Masterminds/semver"
)

type version struct {
	semver.Version
}

func parse(t string) (*version, error) {
	v, err := semver.NewVersion(strings.Replace(t, "_", "+", -1))

	if err != nil {
		return nil, err
	}

	return &version{Version: *v}, nil
}

func (v *version) Pre() int64 {
	r := v.Metadata()

	if r == "" {
		return 0
	}

	res, err := strconv.Atoi(strings.TrimPrefix(r, "pre"))

	if err != nil {
		return 0
	}

	return int64(res)
}

func (v *version) RC() int64 {
	r := v.Prerelease()

	if r == "" {
		return 0
	}

	res, err := strconv.Atoi(strings.TrimPrefix(r, "rc"))

	if err != nil {
		return 0
	}

	return int64(res)
}

func (v *version) String() string {
	return strings.Replace(fmt.Sprintf("v%s", v.Version.String()), "+", "_", -1)
}

func (v *version) Compare(v2 *version) int {
	if v.Major() == v2.Major() && v.Minor() == v2.Minor() &&
		v.Patch() == v2.Patch() && v.RC() != v2.RC() {
		if v.RC()*v2.RC() == 0 {
			return int(v2.RC() - v.RC())
		}

		return int(v.RC() - v2.RC())
	}

	return v.Version.Compare(&v2.Version)
}

func (v *version) IncMajor() {
	v.Version = v.Version.IncMajor()
}

func (v *version) IncMinor() {
	v.Version = v.Version.IncMinor()
}

func (v *version) IncPatch() {
	v.Version = v.Version.IncPatch()
}

func (v *version) IncPre() {
	pre := v.Pre()

	if pre == 0 {
		v.IncRC()
	}

	v.Version, _ = v.SetMetadata(fmt.Sprintf("pre%d", pre+1))
}

func (v *version) IncRC() {
	if v.RC() == 0 {
		v.IncPatch()
	}

	v.Version, _ = v.SetPrerelease(fmt.Sprintf("rc%d", v.RC()+1))
	v.Version, _ = v.SetMetadata("")
}

func incrementVersionFromCommits(v *version, messages []string) bool {
	var bumped bool

	for _, message := range messages {
		if strings.Contains(message, "bump-major") {
			v.IncMajor()
			bumped = true
		}

		if strings.Contains(message, "bump-minor") {
			v.IncMinor()
			bumped = true
		}
	}

	return bumped
}
