// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package ifacetest

import (
	"context"
	"sync"

	"github.com/canonical/workshop/internal/interfaces"
	"github.com/canonical/workshop/internal/osutil"
	"github.com/canonical/workshop/internal/sdk"
	"github.com/canonical/workshop/internal/workshop"
)

// TestSecurityBackend is a security backend intended for testing.
type TestSecurityBackend struct {
	BackendName interfaces.SecuritySystem
	// SetupCalls stores information about all calls to Setup
	SetupCalls []TestSetupCall
	// RemoveCalls stores information about all calls to Remove
	RemoveCalls []string
	// SetupCallback is an callback that is optionally called in Setup
	SetupCallback func(context context.Context, sdkRef sdk.Ref, repo *interfaces.Repository) error
	// SupportsPlugCallback is optionally called by TestSandbox.SupportsPlug
	SupportsPlugCallback func(iface interfaces.Interface, plug *sdk.PlugInfo, w *workshop.Workshop) error
	// SupportsSlotCallback is optionally called by TestSandbox.SupportsSlot
	SupportsSlotCallback func(iface interfaces.Interface, slot *sdk.SlotInfo, w *workshop.Workshop) error
	// RemoveCallback is a callback that is optionally called in Remove
	RemoveCallback func(sdkName string) error
	lock           sync.Mutex
	// SandboxFeaturesCallback is a callback that is optionally called in SandboxFeatures
	SandboxFeaturesCallback func() []string
}

// TestSetupCall stores details about calls to TestSecurityBackend.Setup
type TestSetupCall struct {
	// SdkInfo is a copy of the sdkRef argument to a particular call to Setup
	SdkRef sdk.Ref
}

type TestSandbox struct {
	Backend  *TestSecurityBackend
	Workshop *workshop.Workshop
}

// Initialize does nothing.
func (b *TestSecurityBackend) Initialize() error {
	return nil
}

// Name returns the name of the security backend.
func (b *TestSecurityBackend) Name() interfaces.SecuritySystem {
	return b.BackendName
}

// Setup records information about the call and calls the setup callback if one is defined.
func (b *TestSecurityBackend) Setup(context context.Context, sdkRef sdk.Ref, repo *interfaces.Repository) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.SetupCalls = append(b.SetupCalls, TestSetupCall{SdkRef: sdkRef})
	if b.SetupCallback == nil {
		return nil
	}
	return b.SetupCallback(context, sdkRef, repo)
}

// Remove records information about the call and calls the remove callback if one is defined
func (b *TestSecurityBackend) Remove(context context.Context, sdkRef sdk.Ref) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.RemoveCalls = append(b.RemoveCalls, sdkRef.Sdk)
	if b.RemoveCallback == nil {
		return nil
	}
	return b.RemoveCallback(sdkRef.Sdk)
}

func (b *TestSecurityBackend) NewSpecification(user string, sdk string) (interfaces.Specification, error) {
	usr, err := osutil.UserLookup(user)
	if err != nil {
		return nil, err
	}
	return &Specification{user: usr, sdk: sdk}, nil
}

func (b *TestSecurityBackend) NewSandbox(w *workshop.Workshop) interfaces.Sandbox {
	return &TestSandbox{
		Backend:  b,
		Workshop: w,
	}
}

func (s *TestSandbox) SupportsPlug(iface interfaces.Interface, plug *sdk.PlugInfo) error {
	if s.Backend.SupportsPlugCallback == nil {
		return nil
	}
	return s.Backend.SupportsPlugCallback(iface, plug, s.Workshop)
}

func (s *TestSandbox) SupportsSlot(iface interfaces.Interface, slot *sdk.SlotInfo) error {
	if s.Backend.SupportsSlotCallback == nil {
		return nil
	}
	return s.Backend.SupportsSlotCallback(iface, slot, s.Workshop)
}

func (b *TestSecurityBackend) SandboxFeatures() []string {
	if b.SandboxFeaturesCallback == nil {
		return nil
	}
	return b.SandboxFeaturesCallback()
}
