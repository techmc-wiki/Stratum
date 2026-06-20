package python

import "testing"

func TestIsValidManagerType(t *testing.T) {
	tests := []struct {
		managerType ManagerType
		expected    bool
	}{
		{ManagerVenv, true},
		{ManagerUV, true},
		{ManagerType("invalid"), false},
		{ManagerType(""), false},
		{ManagerType("pip"), false},
	}

	for _, tc := range tests {
		t.Run(string(tc.managerType), func(t *testing.T) {
			actual := IsValidManagerType(tc.managerType)
			if actual != tc.expected {
				t.Errorf("IsValidManagerType(%q) = %v, expected %v", tc.managerType, actual, tc.expected)
			}
		})
	}
}
