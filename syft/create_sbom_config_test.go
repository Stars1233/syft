package syft

import (
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/scylladb/go-set/strset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/syft/internal/task"
	"github.com/anchore/syft/syft/artifact"
	"github.com/anchore/syft/syft/cataloging"
	"github.com/anchore/syft/syft/cataloging/filecataloging"
	"github.com/anchore/syft/syft/cataloging/pkgcataloging"
	"github.com/anchore/syft/syft/file"
	"github.com/anchore/syft/syft/pkg"
	"github.com/anchore/syft/syft/source"
)

var _ pkg.Cataloger = (*dummyCataloger)(nil)

type dummyCataloger struct {
	name string
}

func newDummyCataloger(name string) pkg.Cataloger {
	return dummyCataloger{name: name}
}

func (d dummyCataloger) Name() string {
	return d.name
}

func (d dummyCataloger) Catalog(_ context.Context, _ file.Resolver) ([]pkg.Package, []artifact.Relationship, error) {
	return nil, nil, nil
}

func TestCreateSBOMConfig_makeTaskGroups(t *testing.T) {
	pkgIntersect := func(intersect ...string) []string {
		var sets []*strset.Set
		for _, s := range intersect {
			sets = append(sets, strset.New(pkgCatalogerNamesWithTagOrName(t, s)...))
		}

		intersectSet := strset.Intersection(sets...)

		slice := intersectSet.List()

		sort.Strings(slice)

		return slice
	}

	addTo := func(slice []string, add ...string) []string {
		slice = append(slice, add...)
		sort.Strings(slice)
		return slice
	}

	imgSrc := source.Description{
		Metadata: source.ImageMetadata{},
	}

	dirSrc := source.Description{
		Metadata: source.DirectoryMetadata{},
	}

	fileSrc := source.Description{
		Metadata: source.FileMetadata{},
	}

	tests := []struct {
		name          string
		src           source.Description
		cfg           *CreateSBOMConfig
		wantTaskNames [][]string
		wantManifest  *catalogerManifest
		wantErr       require.ErrorAssertionFunc
	}{
		{
			name: "default catalogers for image source",
			src:  imgSrc,
			cfg:  DefaultCreateSBOMConfig(),
			wantTaskNames: [][]string{
				environmentCatalogerNames(),
				pkgCatalogerNamesWithTagOrName(t, "image"),
				fileCatalogerNames(),
				relationshipCatalogerNames(),
				unknownsTaskNames(),
				osFeatureDetectionTaskNames(),
			},
			wantManifest: &catalogerManifest{
				Requested: cataloging.SelectionRequest{
					DefaultNamesOrTags: []string{"image", "file"},
				},
				Used: flatten(pkgCatalogerNamesWithTagOrName(t, "image"), fileCatalogerNames()),
			},
			wantErr: require.NoError,
		},
		{
			name: "default catalogers for directory source",
			src:  dirSrc,
			cfg:  DefaultCreateSBOMConfig(),
			wantTaskNames: [][]string{
				environmentCatalogerNames(),
				pkgCatalogerNamesWithTagOrName(t, "directory"),
				fileCatalogerNames(),
				relationshipCatalogerNames(),
				unknownsTaskNames(),
				osFeatureDetectionTaskNames(),
			},
			wantManifest: &catalogerManifest{
				Requested: cataloging.SelectionRequest{
					DefaultNamesOrTags: []string{"directory", "file"},
				},
				Used: flatten(pkgCatalogerNamesWithTagOrName(t, "directory"), fileCatalogerNames()),
			},
			wantErr: require.NoError,
		},
		{
			// note, the file source acts like a directory scan
			name: "default catalogers for file source",
			src:  fileSrc,
			cfg:  DefaultCreateSBOMConfig(),
			wantTaskNames: [][]string{
				environmentCatalogerNames(),
				pkgCatalogerNamesWithTagOrName(t, "directory"),
				fileCatalogerNames(),
				relationshipCatalogerNames(),
				unknownsTaskNames(),
				osFeatureDetectionTaskNames(),
			},
			wantManifest: &catalogerManifest{
				Requested: cataloging.SelectionRequest{
					DefaultNamesOrTags: []string{"directory", "file"},
				},
				Used: flatten(pkgCatalogerNamesWithTagOrName(t, "directory"), fileCatalogerNames()),
			},
			wantErr: require.NoError,
		},
		{
			name: "no file digest cataloger",
			src:  imgSrc,
			cfg:  DefaultCreateSBOMConfig().WithCatalogerSelection(cataloging.NewSelectionRequest().WithRemovals("digest")),
			wantTaskNames: [][]string{
				environmentCatalogerNames(),
				pkgCatalogerNamesWithTagOrName(t, "image"),
				fileCatalogerNames("file-metadata", "content", "binary-metadata"),
				relationshipCatalogerNames(),
				unknownsTaskNames(),
				osFeatureDetectionTaskNames(),
			},
			wantManifest: &catalogerManifest{
				Requested: cataloging.SelectionRequest{
					DefaultNamesOrTags: []string{"image", "file"},
					RemoveNamesOrTags:  []string{"digest"},
				},
				Used: flatten(pkgCatalogerNamesWithTagOrName(t, "image"), fileCatalogerNames("file-metadata", "content", "binary-metadata")),
			},
			wantErr: require.NoError,
		},
		{
			name: "select no file catalogers",
			src:  imgSrc,
			cfg:  DefaultCreateSBOMConfig().WithCatalogerSelection(cataloging.NewSelectionRequest().WithRemovals("file")),
			wantTaskNames: [][]string{
				environmentCatalogerNames(),
				pkgCatalogerNamesWithTagOrName(t, "image"),
				nil, // note: there is a file cataloging group, with no items in it
				relationshipCatalogerNames(),
				unknownsTaskNames(),
				osFeatureDetectionTaskNames(),
			},
			wantManifest: &catalogerManifest{
				Requested: cataloging.SelectionRequest{
					DefaultNamesOrTags: []string{"image", "file"},
					RemoveNamesOrTags:  []string{"file"},
				},
				Used: pkgCatalogerNamesWithTagOrName(t, "image"),
			},
			wantErr: require.NoError,
		},
		{
			name: "select all file catalogers",
			src:  imgSrc,
			cfg:  DefaultCreateSBOMConfig().WithFilesConfig(filecataloging.DefaultConfig().WithSelection(file.AllFilesSelection)),
			wantTaskNames: [][]string{
				environmentCatalogerNames(),
				// note: there is a single group of catalogers for pkgs and files
				append(
					pkgCatalogerNamesWithTagOrName(t, "image"),
					fileCatalogerNames()...,
				),
				relationshipCatalogerNames(),
				unknownsTaskNames(),
				osFeatureDetectionTaskNames(),
			},
			wantManifest: &catalogerManifest{
				Requested: cataloging.SelectionRequest{
					DefaultNamesOrTags: []string{"image", "file"},
				},
				Used: flatten(pkgCatalogerNamesWithTagOrName(t, "image"), fileCatalogerNames()),
			},
			wantErr: require.NoError,
		},
		{
			name: "user-provided persistent cataloger is always run (image)",
			src:  imgSrc,
			cfg: DefaultCreateSBOMConfig().WithCatalogers(
				pkgcataloging.NewAlwaysEnabledCatalogerReference(newDummyCataloger("persistent")),
			),
			wantTaskNames: [][]string{
				environmentCatalogerNames(),
				addTo(pkgCatalogerNamesWithTagOrName(t, "image"), "persistent"),
				fileCatalogerNames(),
				relationshipCatalogerNames(),
				unknownsTaskNames(),
				osFeatureDetectionTaskNames(),
			},
			wantManifest: &catalogerManifest{
				Requested: cataloging.SelectionRequest{
					DefaultNamesOrTags: []string{"image", "file"},
				},
				Used: flatten(addTo(pkgCatalogerNamesWithTagOrName(t, "image"), "persistent"), fileCatalogerNames()),
			},
			wantErr: require.NoError,
		},
		{
			name: "user-provided persistent cataloger is always run (directory)",
			src:  dirSrc,
			cfg: DefaultCreateSBOMConfig().WithCatalogers(
				pkgcataloging.NewAlwaysEnabledCatalogerReference(newDummyCataloger("persistent")),
			),
			wantTaskNames: [][]string{
				environmentCatalogerNames(),
				addTo(pkgCatalogerNamesWithTagOrName(t, "directory"), "persistent"),
				fileCatalogerNames(),
				relationshipCatalogerNames(),
				unknownsTaskNames(),
				osFeatureDetectionTaskNames(),
			},
			wantManifest: &catalogerManifest{
				Requested: cataloging.SelectionRequest{
					DefaultNamesOrTags: []string{"directory", "file"},
				},
				Used: flatten(addTo(pkgCatalogerNamesWithTagOrName(t, "directory"), "persistent"), fileCatalogerNames()),
			},
			wantErr: require.NoError,
		},
		{
			name: "user-provided persistent cataloger is always run (user selection does not affect this)",
			src:  imgSrc,
			cfg: DefaultCreateSBOMConfig().WithCatalogers(
				pkgcataloging.NewAlwaysEnabledCatalogerReference(newDummyCataloger("persistent")),
			).WithCatalogerSelection(cataloging.NewSelectionRequest().WithSubSelections("javascript")),
			wantTaskNames: [][]string{
				environmentCatalogerNames(),
				addTo(pkgIntersect("image", "javascript"), "persistent"),
				fileCatalogerNames(),
				relationshipCatalogerNames(),
				unknownsTaskNames(),
				osFeatureDetectionTaskNames(),
			},
			wantManifest: &catalogerManifest{
				Requested: cataloging.SelectionRequest{
					DefaultNamesOrTags: []string{"image", "file"},
					SubSelectTags:      []string{"javascript"},
				},
				Used: flatten(addTo(pkgIntersect("image", "javascript"), "persistent"), fileCatalogerNames()),
			},
			wantErr: require.NoError,
		},
		{
			name: "user-provided cataloger runs when selected",
			src:  imgSrc,
			cfg: DefaultCreateSBOMConfig().WithCatalogers(
				pkgcataloging.NewCatalogerReference(newDummyCataloger("user-provided"), []string{"image"}),
			),
			wantTaskNames: [][]string{
				environmentCatalogerNames(),
				addTo(pkgCatalogerNamesWithTagOrName(t, "image"), "user-provided"),
				fileCatalogerNames(),
				relationshipCatalogerNames(),
				unknownsTaskNames(),
				osFeatureDetectionTaskNames(),
			},
			wantManifest: &catalogerManifest{
				Requested: cataloging.SelectionRequest{
					DefaultNamesOrTags: []string{"image", "file"},
				},
				Used: flatten(addTo(pkgCatalogerNamesWithTagOrName(t, "image"), "user-provided"), fileCatalogerNames()),
			},
			wantErr: require.NoError,
		},
		{
			name: "user-provided cataloger NOT run when NOT selected",
			src:  imgSrc,
			cfg: DefaultCreateSBOMConfig().WithCatalogers(
				pkgcataloging.NewCatalogerReference(newDummyCataloger("user-provided"), []string{"bogus-selector-will-never-be-used"}),
			),
			wantTaskNames: [][]string{
				environmentCatalogerNames(),
				pkgCatalogerNamesWithTagOrName(t, "image"),
				fileCatalogerNames(),
				relationshipCatalogerNames(),
				unknownsTaskNames(),
				osFeatureDetectionTaskNames(),
			},
			wantManifest: &catalogerManifest{
				Requested: cataloging.SelectionRequest{
					DefaultNamesOrTags: []string{"image", "file"},
				},
				Used: flatten(pkgCatalogerNamesWithTagOrName(t, "image"), fileCatalogerNames()),
			},
			wantErr: require.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}

			// sanity check
			require.NotEmpty(t, tt.wantTaskNames)

			// test the subject
			gotTasks, gotManifest, err := tt.cfg.makeTaskGroups(tt.src)
			tt.wantErr(t, err)
			if err != nil {
				return
			}

			gotNames := taskGroupNames(gotTasks)

			if d := cmp.Diff(
				tt.wantTaskNames,
				gotNames,
				// order within a group does not matter
				cmpopts.SortSlices(func(a, b string) bool {
					return a < b
				}),
			); d != "" {
				t.Errorf("mismatched task group names (-want +got):\n%s", d)
			}

			if d := cmp.Diff(tt.wantManifest, gotManifest); d != "" {
				t.Errorf("mismatched cataloger manifest (-want +got):\n%s", d)
			}
		})
	}
}

func pkgCatalogerNamesWithTagOrName(t *testing.T, token string) []string {
	var names []string
	cfg := task.DefaultCatalogingFactoryConfig()
	for _, factory := range task.DefaultPackageTaskFactories() {
		cat := factory(cfg)

		name := cat.Name()

		if selector, ok := cat.(task.Selector); ok {
			if selector.HasAllSelectors(token) {
				names = append(names, name)
				continue
			}
		}
		if name == token {
			names = append(names, name)
		}
	}

	// these thresholds are arbitrary but should be large enough to catch any major changes
	switch token {
	case "image":
		require.Greater(t, len(names), 18, "minimum cataloger sanity check failed token")
	case "directory":
		require.Greater(t, len(names), 25, "minimum cataloger sanity check failed token")
	default:
		require.Greater(t, len(names), 0, "minimum cataloger sanity check failed token")
	}

	sort.Strings(names)
	return names
}

func fileCatalogerNames(tokens ...string) []string {
	var names []string
	cfg := task.DefaultCatalogingFactoryConfig()
topLoop:
	for _, factory := range task.DefaultFileTaskFactories() {
		cat := factory(cfg)

		if cat == nil {
			continue
		}

		name := cat.Name()

		if len(tokens) == 0 {
			names = append(names, name)
			continue
		}

		for _, token := range tokens {
			if selector, ok := cat.(task.Selector); ok {
				if selector.HasAllSelectors(token) {
					names = append(names, name)
					continue topLoop
				}

			}
			if name == token {
				names = append(names, name)
			}
		}
	}

	sort.Strings(names)
	return names
}

func flatten(lists ...[]string) []string {
	var final []string
	for _, lst := range lists {
		final = append(final, lst...)
	}
	sort.Strings(final)
	return final
}

func relationshipCatalogerNames() []string {
	return []string{"relationships-cataloger"}
}

func unknownsTaskNames() []string {
	return []string{"unknowns-labeler"}
}

func osFeatureDetectionTaskNames() []string {
	return []string{"os-feature-detection"}
}

func environmentCatalogerNames() []string {
	return []string{"environment-cataloger"}
}

func taskGroupNames(groups [][]task.Task) [][]string {
	var names [][]string
	for _, group := range groups {
		var groupNames []string
		for _, tsk := range group {
			groupNames = append(groupNames, tsk.Name())
		}
		names = append(names, groupNames)
	}
	return names
}

func Test_replaceDefaultTagReferences(t *testing.T) {

	tests := []struct {
		name string
		lst  []string
		want []string
	}{
		{
			name: "no default tag",
			lst:  []string{"foo", "bar"},
			want: []string{"foo", "bar"},
		},
		{
			name: "replace default tag",
			lst:  []string{"foo", "default", "bar"},
			want: []string{"foo", "replacement", "bar"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, replaceDefaultTagReferences([]string{"replacement"}, tt.lst))
		})
	}
}

func Test_findDefaultTag(t *testing.T) {

	tests := []struct {
		name    string
		src     source.Description
		want    []string
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "image",
			src: source.Description{
				Metadata: source.ImageMetadata{},
			},
			want: []string{pkgcataloging.ImageTag, filecataloging.FileTag},
		},
		{
			name: "directory",
			src: source.Description{
				Metadata: source.DirectoryMetadata{},
			},
			want: []string{pkgcataloging.DirectoryTag, filecataloging.FileTag},
		},
		{
			name: "file",
			src: source.Description{
				Metadata: source.FileMetadata{},
			},
			want: []string{pkgcataloging.DirectoryTag, filecataloging.FileTag}, // not a mistake...
		},
		{
			name: "unknown",
			src: source.Description{
				Metadata: struct{}{},
			},
			wantErr: require.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			got, err := findDefaultTags(tt.src)
			tt.wantErr(t, err)
			if err != nil {
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCreateSBOMConfig_validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *CreateSBOMConfig
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "incompatible ExcludeBinaryPackagesWithFileOwnershipOverlap selection",
			cfg: DefaultCreateSBOMConfig().
				WithRelationshipsConfig(
					cataloging.DefaultRelationshipsConfig().
						WithExcludeBinaryPackagesWithFileOwnershipOverlap(true).
						WithPackageFileOwnershipOverlap(false),
				),
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = assert.NoError
			}
			tt.wantErr(t, tt.cfg.validate())
		})
	}
}
