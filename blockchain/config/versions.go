// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The NogoChain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package config

import (
	"errors"
	"fmt"
	"strings"
)

// Version represents a semantic version
type Version struct {
	Major uint32 `json:"major"`
	Minor uint32 `json:"minor"`
	Patch uint32 `json:"patch"`
	Name  string `json:"name"`
}

// NewVersion creates a new version
func NewVersion(major, minor, patch uint32, name string) Version {
	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Name:  name,
	}
}

// String returns the string representation
func (v Version) String() string {
	if v.Name != "" {
		return fmt.Sprintf("%d.%d.%d-%s", v.Major, v.Minor, v.Patch, v.Name)
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare compares two versions
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major > other.Major {
			return 1
		}
		return -1
	}

	if v.Minor != other.Minor {
		if v.Minor > other.Minor {
			return 1
		}
		return -1
	}

	if v.Patch != other.Patch {
		if v.Patch > other.Patch {
			return 1
		}
		return -1
	}

	return 0
}

// IsGreater returns true if this version is greater than other
func (v Version) IsGreater(other Version) bool {
	return v.Compare(other) > 0
}

// IsGreaterOrEqual returns true if this version is greater or equal to other
func (v Version) IsGreaterOrEqual(other Version) bool {
	return v.Compare(other) >= 0
}

// IsLess returns true if this version is less than other
func (v Version) IsLess(other Version) bool {
	return v.Compare(other) < 0
}

// IsCompatible checks if versions are compatible (same major version)
func (v Version) IsCompatible(other Version) bool {
	return v.Major == other.Major
}

// ParseVersion parses a version string
func ParseVersion(s string) (Version, error) {
	parts := strings.Split(s, ".")
	if len(parts) < 3 {
		return Version{}, ErrInvalidVersionFormat
	}

	var major, minor, patch uint32
	fmt.Sscanf(parts[0], "%d", &major)
	fmt.Sscanf(parts[1], "%d", &minor)

	nameParts := strings.SplitN(parts[2], "-", 2)
	fmt.Sscanf(nameParts[0], "%d", &patch)

	name := ""
	if len(nameParts) > 1 {
		name = nameParts[1]
	}

	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Name:  name,
	}, nil
}

// ProtocolVersion defines protocol version information
type ProtocolVersion struct {
	Version
	ActivationHeight uint64 `json:"activationHeight"`
	Mandatory        bool   `json:"mandatory"`
	MinPeerVersion   string `json:"minPeerVersion"`
}

// VersionChecker checks version compatibility
type VersionChecker struct {
	current   Version
	minPeer   Version
	mandatory bool
}

// NewVersionChecker creates a new version checker
func NewVersionChecker(current, minPeer Version, mandatory bool) *VersionChecker {
	return &VersionChecker{
		current:   current,
		minPeer:   minPeer,
		mandatory: mandatory,
	}
}

// IsCompatible checks if a peer version is compatible
func (c *VersionChecker) IsCompatible(peerVersion Version) bool {
	if c.mandatory {
		return peerVersion.IsGreaterOrEqual(c.minPeer) && peerVersion.Major == c.current.Major
	}
	return peerVersion.Major == c.current.Major
}

// GetCurrentVersion returns the current version
func (c *VersionChecker) GetCurrentVersion() Version {
	return c.current
}

// GetMinPeerVersion returns the minimum peer version
func (c *VersionChecker) GetMinPeerVersion() Version {
	return c.minPeer
}

// IsMandatory returns true if version check is mandatory
func (c *VersionChecker) IsMandatory() bool {
	return c.mandatory
}

// Error definitions
var (
	ErrInvalidVersionFormat = errors.New("invalid version format")
)
