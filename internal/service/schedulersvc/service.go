package schedulersvc

import (
	"github.com/stratummc/stratum/internal/domain/resourcepolicy"
	"github.com/stratummc/stratum/internal/domain/session"
)

type Service struct{ Policy resourcepolicy.Policy }

func (s Service) Decide(usage resourcepolicy.Usage, requested session.Type) resourcepolicy.Decision {
	return resourcepolicy.Evaluate(s.Policy, usage, requested)
}
