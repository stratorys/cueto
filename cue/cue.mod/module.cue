// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// CUE module root. Making cue/ a module turns it into an import root so exports
// are portable. The diagram schema lives in the diagram/ subpackage and is
// imported as "github.com/stratorys/cueto/diagram"; the default project (data.cue)
// is package main and imports it.
module: "github.com/stratorys/cueto"
language: version: "v0.17.0"
