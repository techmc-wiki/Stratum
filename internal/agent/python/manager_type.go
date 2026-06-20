package python

type ManagerType string

const (
	ManagerVenv ManagerType = "venv"
	ManagerUV   ManagerType = "uv"
)

func IsValidManagerType(t ManagerType) bool {
	switch t {
	case ManagerVenv, ManagerUV:
		return true
	}
	return false
}
