package navidrome

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// RadioSpec is the YAML format for Navidrome internet radio stations.
type RadioSpec struct {
	Stations []RadioStation `yaml:"stations"`
}

// ReadRadioSpec reads and validates an internet radio YAML spec.
func ReadRadioSpec(r io.Reader) (RadioSpec, error) {
	var spec RadioSpec
	if err := yaml.NewDecoder(r).Decode(&spec); err != nil {
		return RadioSpec{}, err
	}
	if len(spec.Stations) == 0 {
		return RadioSpec{}, fmt.Errorf("radio stations are required")
	}
	urls := make(map[string]struct{}, len(spec.Stations))
	for i, station := range spec.Stations {
		if station.Name == "" || station.StreamURL == "" {
			return RadioSpec{}, fmt.Errorf("station %d needs name and stream_url", i+1)
		}
		if _, found := urls[station.StreamURL]; found {
			return RadioSpec{}, fmt.Errorf("duplicate stream_url %q", station.StreamURL)
		}
		urls[station.StreamURL] = struct{}{}
	}
	return spec, nil
}

// WriteRadioSpec writes an internet radio YAML spec.
func WriteRadioSpec(w io.Writer, spec RadioSpec) error {
	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	if err := encoder.Encode(spec); err != nil {
		_ = encoder.Close()
		return err
	}
	return encoder.Close()
}
