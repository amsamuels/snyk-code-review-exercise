package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/gorilla/mux"
)

//  idea: dir structure suggestion:
// api/               → HTTP handler
// resolver/          → Dependency resolution logic
// fetcher/           → Network layer and caching
// models/            → Struct definitions
// interfaces/        → Shared interface declarations

// review: introducing interface to api.New for dependency fetching abstraction
// idea: makes it mockable and testable

// review: use a Server struct for better encapsulation and testability

func New() http.Handler {
	router := mux.NewRouter()
	router.Handle("/package/{package}/{version}", http.HandlerFunc(packageHandler))
	return router
}

type npmPackageMetaResponse struct {
	Versions map[string]npmPackageResponse `json:"versions"`
}

type npmPackageResponse struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
}

type NpmPackageVersion struct {
	Name         string                        `json:"name"`
	Version      string                        `json:"version"`
	Dependencies map[string]*NpmPackageVersion `json:"dependencies"`
}

func packageHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pkgName := vars["package"]
	pkgVersion := vars["version"]

	// review: Validate that pkgName and pkgVersion are not empty and well-formed
	// review: Validate that pkgVersion is a valid semver string (use semver lib)
	// idea: Support content negotiation using the Accept header (JSON, plain text, etc.)

	rootPkg := &NpmPackageVersion{Name: pkgName, Dependencies: map[string]*NpmPackageVersion{}}
	if err := resolveDependencies(rootPkg, pkgVersion); err != nil {
		// review: Use proper structured logging instead of println
		println(err.Error())
		// review: Return meaningful HTTP error responses (400 for bad input, 404 for not found, etc.) & add useful error message for user
		w.WriteHeader(500)
		return
	}

	stringified, err := json.MarshalIndent(rootPkg, "", "  ")
	if err != nil {
		// review: Log JSON marshalling error properly
		println(err.Error())
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	// Ignoring ResponseWriter errors
	// review: check error returned by w.Write, do not ignore, log them
	_, _ = w.Write(stringified)
}

func resolveDependencies(pkg *NpmPackageVersion, versionConstraint string) error {
	// review: Check for circular dependency using visited map
	pkgMeta, err := fetchPackageMeta(pkg.Name)
	if err != nil {
		return err
	}
	concreteVersion, err := highestCompatibleVersion(versionConstraint, pkgMeta)
	if err != nil {
		return err
	}
	pkg.Version = concreteVersion
	// review: Add caching here so we don’t repeatedly fetch the same package
	// idea: Consider using a memoizing struct or service to manage cache with expiration
	npmPkg, err := fetchPackage(pkg.Name, pkg.Version)
	if err != nil {
		return err
	}

	// review: Optionally fetch dependencies concurrently with sync.WaitGroup or errgroup
	// idea: Add max depth control to avoid runaway recursion
	for dependencyName, dependencyVersionConstraint := range npmPkg.Dependencies {
		dep := &NpmPackageVersion{Name: dependencyName, Dependencies: map[string]*NpmPackageVersion{}}
		pkg.Dependencies[dependencyName] = dep
		if err := resolveDependencies(dep, dependencyVersionConstraint); err != nil {
			// review: Optionally log and continue rather than failing entire tree
			// idea: Collect and return aggregated errors from all sub-dependencies
			return err
		}
	}
	return nil
}

func highestCompatibleVersion(constraintStr string, versions *npmPackageMetaResponse) (string, error) {
	constraint, err := semver.NewConstraint(constraintStr)
	if err != nil {
		return "", err
	}
	filtered := filterCompatibleVersions(constraint, versions)
	sort.Sort(filtered)
	if len(filtered) == 0 {
		return "", errors.New("no compatible versions found")
	}

	// idea: Allow caller to choose between latest and earliest compatible versions
	return filtered[len(filtered)-1].String(), nil
}

func filterCompatibleVersions(constraint *semver.Constraints, pkgMeta *npmPackageMetaResponse) semver.Collection {
	var compatible semver.Collection
	for version := range pkgMeta.Versions {
		semVer, err := semver.NewVersion(version)
		if err != nil {
			// idea: Log or collect invalid semver versions for debugging/monitoring
			continue
		}
		if constraint.Check(semVer) {
			compatible = append(compatible, semVer)
		}
	}
	return compatible
}

func fetchPackage(name, version string) (*npmPackageResponse, error) {
	resp, err := http.Get(fmt.Sprintf("https://registry.npmjs.org/%s/%s", name, version))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// review: Handle non-200 HTTP status codes with informative errors
	// idea: Add retry logic and backoff on transient errors or 429 rate-limits
	// idea: Expose basic request timing or failure metrics via Prometheus or logs

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed npmPackageResponse
	// review: check and handle error from json.Unmarshal instead of ignoring it
	_ = json.Unmarshal(body, &parsed)
	return &parsed, nil
}

func fetchPackageMeta(p string) (*npmPackageMetaResponse, error) {
	resp, err := http.Get(fmt.Sprintf("https://registry.npmjs.org/%s", p))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// review: Handle non-200 HTTP status codes with informative errors
	// idea: Add retry logic and backoff on transient errors or 429 rate-limits
	// idea: Expose basic request timing or failure metrics via Prometheus or logs
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed npmPackageMetaResponse
	// review: body is already a []byte, no need to wrap it again
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, err
	}

	return &parsed, nil
}
