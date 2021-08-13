package config

import (
	"fmt"
	"strings"
)

func NewVerifyError() verfiyError {
	return verfiyError{
		errors: []error{},
	}
}

type verfiyError struct {
	errors []error
}

func (vErr *verfiyError) Error() string {
	var sb strings.Builder
	sb.WriteString("The following errors have been found: ")
	for _, err := range vErr.errors {
		sb.WriteString("\n")
		sb.WriteString(err.Error())
	}
	return sb.String()
}

func (err *verfiyError) HasErrors() bool {
	return len(err.errors) > 0
}

func (vErr *verfiyError) Add(err error) {
	vErr.errors = append(vErr.errors, err)
}

type DuplicateDomain struct {
	Cfg1Path string
	Cfg2Path string
	Domain   string
}

func (err *DuplicateDomain) Error() string {
	return fmt.Sprintf("'%s' has been found in %s and %s", err.Domain, err.Cfg1Path, err.Cfg2Path)
}

func VerifyConfigs(cfgs []ServerConfig) verfiyError {
	vErrors := NewVerifyError()
	domains := make(map[string]int)
	for index, cfg := range cfgs {
		if len(cfg.Domains) == 0 {
			err := fmt.Errorf("'domains' is not allowed to be empty in %s", cfg.FilePath)
			vErrors.Add(err)
		}
		if cfg.ProxyTo == "" {
			err := fmt.Errorf("'proxyTo' is not allowed to be empty in %s", cfg.FilePath)
			vErrors.Add(err)
		}
		for _, domain := range cfg.Domains {
			otherIndex, ok := domains[domain]
			if ok {
				err := &DuplicateDomain{
					Domain:   domain,
					Cfg1Path: cfg.FilePath,
					Cfg2Path: cfgs[otherIndex].FilePath,
				}
				vErrors.Add(err)
				continue
			}
			domains[domain] = index
		}
	}
	return vErrors
}
