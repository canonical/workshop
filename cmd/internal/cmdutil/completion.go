// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2018 Canonical Ltd
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

package cmdutil

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func CompleteChoices(choices ...string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return choices, cobra.ShellCompDirectiveNoFileComp
	}
}

func CompleteEnv(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	environ := os.Environ()
	names := make([]string, 0, len(environ))
	for _, e := range environ {
		if strings.HasPrefix(e, "_") && !strings.HasPrefix(toComplete, "_") {
			continue
		}
		name, _, _ := strings.Cut(e, "=")
		names = append(names, name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
