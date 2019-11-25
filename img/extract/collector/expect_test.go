package collector

import (
	"fmt"
	"path/filepath"

	"github.com/ulmenhaus/env/img/explore/models"
)

var (
	fixturesPkgPath = "github.com/ulmenhaus/env/img/extract/tests/fixtures"
	sourcePkgPath   = "github.com/ulmenhaus/env/img/extract/tests/fixtures/source"
	targetPkgPath   = "github.com/ulmenhaus/env/img/extract/tests/fixtures/target"
)

var nodeTestCases = []struct {
	name string
	node models.EncodedNode
}{
	{
		name: "single-line-prv-const",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "singleLinePrvConst"),
				Kind:        KindConst,
				DisplayName: "target.singleLinePrvConst",
				Description: "a private const defined on a single line\n",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 94,
					Start:  94,
					End:    116,
				},
			},
			Public: false,
		},
	},
	{
		name: "single-line-pub-const",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "SingleLinePubConst"),
				Kind:        KindConst,
				DisplayName: "target.SingleLinePubConst",
				Description: "a private const defined on a single line\n",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 167,
					Start:  167,
					End:    189,
				},
			},
			Public: true,
		},
	},
	{
		name: "multi-line-prv-const",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "multiLinePrvConst"),
				Kind:        KindConst,
				DisplayName: "target.multiLinePrvConst",
				Description: "a private const defined as a part of a multi-line definition\n",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 244,
					Start:  244,
					End:    265,
				},
			},
			Public: false,
		},
	},
	{
		name: "multi-line-pub-const",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "MultiLinePubConst"),
				Kind:        KindConst,
				DisplayName: "target.MultiLinePubConst",
				Description: "a public const defined as a part of a multi-line definition\n",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 331,
					Start:  331,
					End:    352,
				},
			},
			Public: true,
		},
	},
	{
		name: "single-line-prv-var",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "singleLinePrvVar"),
				Kind:        KindVar,
				DisplayName: "target.singleLinePrvVar",
				Description: "a private var defined on a single line\n",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 423,
					Start:  423,
					End:    443,
				},
			},
			Public: false,
		},
	},
	{
		name: "single-line-pub-var",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "SingleLinePubVar"),
				Kind:        KindVar,
				DisplayName: "target.SingleLinePubVar",
				Description: "a private var defined on a single line\n",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 490,
					Start:  490,
					End:    510,
				},
			},
			Public: true,
		},
	},
	{
		name: "multi-line-prv-var",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "multiLinePrvVar"),
				Kind:        KindVar,
				DisplayName: "target.multiLinePrvVar",
				Description: "a private var defined as a part of a multi-line definition\n",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 561,
					Start:  561,
					End:    580,
				},
			},
			Public: false,
		},
	},
	{
		name: "multi-line-pub-var",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "MultiLinePubVar"),
				Kind:        KindVar,
				DisplayName: "target.MultiLinePubVar",
				Description: "a public var defined as a part of a multi-line definition\n",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 644,
					Start:  644,
					End:    663,
				},
			},
			Public: true,
		},
	},
	{
		name: "var-that-is-multiline",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "slVar2var"),
				Kind:        KindVar,
				DisplayName: "target.slVar2var",
				Description: "a var that references other vars\n",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 732,
					Start:  732,
					End:    792,
				},
			},
			Public: false,
		},
	},
	{
		name: "single-line-type",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "SingleLineType"),
				Kind:        KindType,
				DisplayName: "target.SingleLineType",
				Description: "",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 898,
					Start:  898,
					End:    916,
				},
			},
			Public: true,
		},
	},
	{
		name: "multi-line-type",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType"),
				Kind:        KindType,
				DisplayName: "target.MultiLineType",
				Description: "",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 986,
					Start:  986,
					End:    1135,
				},
			},
			Public: true,
		},
	},
	{
		name: "primitive-field",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.SimpleField"),
				Kind:        KindField,
				DisplayName: "target.MultiLineType.SimpleField",
				Description: "a primitive field\n",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 1010,
					Start:  1010,
					End:    1029,
				},
			},
			Public: true,
		},
	},
	{
		name: "composite-field",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.TypeToTypeField"),
				Kind:        KindField,
				DisplayName: "target.MultiLineType.TypeToTypeField",
				Description: "a field that references another type\n",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 1063,
					Start:  1063,
					End:    1093,
				},
			},
			Public: true,
		},
	},
	{
		name: "multi-line-interface",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface"),
				Kind:        KindType,
				DisplayName: "target.MultiLineInterface",
				Description: "",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 1216,
					Start:  1216,
					End:    1507,
				},
			},
			Public: true,
		},
	},
	{
		name: "interface-method",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.SimpleMethod"),
				Kind:        KindMethod,
				DisplayName: "target.MultiLineInterface.SimpleMethod",
				Description: "",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 1312,
					Start:  1312,
					End:    1335,
				},
			},
			Public: true,
		},
	},
	{
		name: "multi-line-function",
		node: models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineFunc"),
				Kind:        KindFunction,
				DisplayName: "target.MultiLineFunc",
				Description: "MultiLineFunc is a multi-line function that references a const, var, type, and field\n",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 1849,
					Start:  1844,
					End:    2066,
				},
			},
			Public: true,
		},
	},
}

var edgeTestCases = []struct {
	name string
	edge models.EncodedEdge
}{
	{
		name: "var-to-const",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "slVar2var"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "singleLinePrvConst"),
		},
	},
	{
		name: "var-to-var",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "slVar2var"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "SingleLinePubVar"),
		},
	},
	{
		name: "field-to-type",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.TypeToTypeField"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "SingleLineType"),
		},
	},
	{
		name: "iface-method-to-input-type",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.CompositeMethod"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "SingleLineType"),
		},
	},
	{
		name: "iface-method-to-return-type",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.CompositeReturn"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "SingleLineType"),
		},
	},

	// References from funtcion
	{
		name: "func-to-const",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineFunc"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "singleLinePrvConst"),
		},
	},
	{
		name: "func-to-var",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineFunc"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "slVar2var"),
		},
	},
	{
		name: "func-to-type",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineFunc"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType"),
		},
	},
	{
		name: "func-to-field",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineFunc"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.SimpleField"),
		},
	},
	{
		name: "func-to-interface",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineFunc"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface"),
		},
	},
	{
		name: "func-to-iface-method",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineFunc"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.SimpleMethod"),
		},
	},
	{
		name: "func-to-func",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineFunc"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "SingleLineFuncCompositeInput"),
		},
	},
	{
		name: "func-to-method",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineFunc"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.MultiLineMethod"),
		},
	},

	// References from method
	{
		name: "method-to-type",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.MultiLineMethod"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType"),
		},
	},
	{
		name: "method-to-method",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.MultiLineMethod"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.SimpleMethod"),
		},
	},

	// References from another file
	{
		name: "if-func-to-type",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "InterFileFunc"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType"),
		},
	},
	{
		name: "if-method-to-method",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", targetPkgPath, "InterFileFunc"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.SimpleMethod"),
		},
	},

	// References from another package
	{
		name: "ip-func-to-type",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", sourcePkgPath, "InterPackageFunc"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType"),
		},
	},
	{
		name: "ip-method-to-method",
		edge: models.EncodedEdge{
			SourceUID: fmt.Sprintf("%s.%s", sourcePkgPath, "InterPackageFunc"),
			DestUID:   fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.SimpleMethod"),
		},
	},
}

var subsystemTestCases = []struct {
	name      string
	subsystem models.EncodedSubsystem
}{
	{
		name: "interface-with-methods",
		subsystem: models.EncodedSubsystem{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.interface"),
				Kind:        KindIface,
				DisplayName: "target.MultiLineInterface.interface",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 1216,
					Start:  1216,
					End:    1507,
				},
			},
			Parts: []string{
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.CompositeMethod"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.CompositeReturn"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.SimpleMethod"),
			},
		},
	},
	{
		name: "struct-with-fields-and-methods",
		subsystem: models.EncodedSubsystem{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.struct"),
				Kind:        KindStruct,
				DisplayName: "target.MultiLineType.struct",
				Location: models.EncodedLocation{
					Path:   filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
					Offset: 986,
					Start:  986,
					End:    1135,
				},
			},
			Parts: []string{
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.MultiLineMethod"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.SimpleField"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.TypeToTypeField"),
			},
		},
	},
	{
		name: "file-with-const-var-type-func",
		subsystem: models.EncodedSubsystem{
			Component: models.Component{
				UID:         fmt.Sprintf("%s/%s", targetPkgPath, "intra.go"),
				Kind:        KindFile,
				DisplayName: fmt.Sprintf("%s/%s", targetPkgPath, "intra.go"),
				Location: models.EncodedLocation{
					Path: filepath.Join(GoPath, "src", targetPkgPath, "intra.go"),
				},
			},
			Parts: []string{
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineFunc"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.interface"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLinePubConst"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLinePubVar"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.struct"),
				fmt.Sprintf("%s.%s", targetPkgPath, "SingleLineFuncCompositeInput"),
				fmt.Sprintf("%s.%s", targetPkgPath, "SingleLineFuncCompositeReturn"),
				fmt.Sprintf("%s.%s", targetPkgPath, "SingleLinePubConst"),
				fmt.Sprintf("%s.%s", targetPkgPath, "SingleLinePubVar"),
				fmt.Sprintf("%s.%s", targetPkgPath, "SingleLineType"),
				fmt.Sprintf("%s.%s", targetPkgPath, "multiLinePrvConst"),
				fmt.Sprintf("%s.%s", targetPkgPath, "multiLinePrvVar"),
				fmt.Sprintf("%s.%s", targetPkgPath, "singleLinePrvConst"),
				fmt.Sprintf("%s.%s", targetPkgPath, "singleLinePrvVar"),
				fmt.Sprintf("%s.%s", targetPkgPath, "slVar2var"),
			},
		},
	},
	{
		name: "package-with-const-var-type-func",
		subsystem: models.EncodedSubsystem{
			Component: models.Component{
				UID:         targetPkgPath,
				Kind:        KindPkg,
				DisplayName: targetPkgPath,
				Location: models.EncodedLocation{
					Path: filepath.Join(GoPath, "src", targetPkgPath),
				},
			},
			Parts: []string{
				fmt.Sprintf("%s.%s", targetPkgPath, "InterFileFunc"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineFunc"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineInterface.interface"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLinePubConst"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLinePubVar"),
				fmt.Sprintf("%s.%s", targetPkgPath, "MultiLineType.struct"),
				fmt.Sprintf("%s.%s", targetPkgPath, "SingleLineFuncCompositeInput"),
				fmt.Sprintf("%s.%s", targetPkgPath, "SingleLineFuncCompositeReturn"),
				fmt.Sprintf("%s.%s", targetPkgPath, "SingleLinePubConst"),
				fmt.Sprintf("%s.%s", targetPkgPath, "SingleLinePubVar"),
				fmt.Sprintf("%s.%s", targetPkgPath, "SingleLineType"),
				fmt.Sprintf("%s.%s", targetPkgPath, "multiLinePrvConst"),
				fmt.Sprintf("%s.%s", targetPkgPath, "multiLinePrvVar"),
				fmt.Sprintf("%s.%s", targetPkgPath, "singleLinePrvConst"),
				fmt.Sprintf("%s.%s", targetPkgPath, "singleLinePrvVar"),
				fmt.Sprintf("%s.%s", targetPkgPath, "slVar2var"),
			},
		},
	},
	{
		name: "directory-with-files",
		subsystem: models.EncodedSubsystem{
			Component: models.Component{
				UID:         targetPkgPath + "/",
				Kind:        KindDir,
				DisplayName: targetPkgPath + "/",
				Location: models.EncodedLocation{
					Path: filepath.Join(GoPath, "src", targetPkgPath),
				},
			},
			Parts: []string{
				fmt.Sprintf("%s/%s", targetPkgPath, "inter.go"),
				fmt.Sprintf("%s/%s", targetPkgPath, "intra.go"),
			},
		},
	},
	{
		name: "directory-with-subdirs",
		subsystem: models.EncodedSubsystem{
			Component: models.Component{
				UID:         fixturesPkgPath + "/",
				Kind:        KindDir,
				DisplayName: fixturesPkgPath + "/",
				Location: models.EncodedLocation{
					Path: filepath.Join(GoPath, "src", fixturesPkgPath),
				},
			},
			Parts: []string{
				sourcePkgPath + "/",
				targetPkgPath + "/",
			},
		},
	},
}
