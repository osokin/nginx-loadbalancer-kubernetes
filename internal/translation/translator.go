// Copyright 2023 f5 Inc. All rights reserved.
// Use of this source code is governed by the Apache
// license that can be found in the LICENSE file.

package translation

type Translator interface {
	translate() (interface{}, error)
}
