package config

import (
	"fmt"
)

type DuplicateDomain struct {
	Cfg1Path string
	Cfg2Path string
	Domain   string
}

func (err *DuplicateDomain) Error() string {
	return fmt.Sprintf("'%s' has been found in %s and %s", err.Domain, err.Cfg1Path, err.Cfg2Path)
}

func VerifyConfigs(cfgs []ServerConfig) []error {
	errors := []error{}
	domains := make(map[string]int)
	for index, cfg := range cfgs {
		for _, domain := range cfg.Domains {
			otherIndex, ok := domains[domain]
			if ok {
				errors = append(errors, &DuplicateDomain{
					Domain:   domain,
					Cfg1Path: cfg.FilePath,
					Cfg2Path: cfgs[otherIndex].FilePath,
				})
				continue
			}
			domains[domain] = index
		}
	}
	return errors
}
