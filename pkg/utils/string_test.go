package utils_test

import (
	"testing"

	"github.com/aspirit/xsocial/pkg/utils"
)

func TestString(t *testing.T) {
	id := utils.StringUtil.UniqueID()
	if len(id) != 16 {
		t.Errorf("UniqueID expect length to 16 but is %d", len(id))
	}
}
