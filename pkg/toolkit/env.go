package toolkit

import (
	"fmt"
	"io"

	"github.com/upfluence/errors"
)

type Masker interface {
	// Mask hides the value from the logs of the following steps of the job.
	Mask(string) error
}

type masker struct {
	w io.Writer
}

func (m masker) Mask(v string) error {
	_, err := fmt.Fprintf(m.w, "::add-mask::%s\n", v)

	return errors.Wrap(err, "mask value")
}

type Variable struct {
	Key   string
	Value string
}

type Environment struct {
	Variables []Variable

	// Secrets are masked before the variables are exported, so they never
	// leak in the logs of the following steps.
	Secrets []string
}

func (e Environment) Export(m Masker, kvw KeyValueWriter) error {
	for _, s := range e.Secrets {
		if err := m.Mask(s); err != nil {
			return errors.Wrap(err, "mask secret")
		}
	}

	for _, v := range e.Variables {
		if err := kvw.WriteKeyValue(v.Key, v.Value); err != nil {
			return errors.Wrapf(err, "write %s", v.Key)
		}
	}

	return nil
}
