// Copyright 2018 The Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package local

import (
	"github.com/vmware/harbor/src/common"
	"github.com/vmware/harbor/src/common/dao"
	"github.com/vmware/harbor/src/common/dao/group"
	"github.com/vmware/harbor/src/common/models"
	"github.com/vmware/harbor/src/common/utils/log"
	"github.com/vmware/harbor/src/ui/promgr"
)

// SecurityContext implements security.Context interface based on database
type SecurityContext struct {
	user *models.User
	pm   promgr.ProjectManager
}

// NewSecurityContext ...
func NewSecurityContext(user *models.User, pm promgr.ProjectManager) *SecurityContext {
	return &SecurityContext{
		user: user,
		pm:   pm,
	}
}

// IsAuthenticated returns true if the user has been authenticated
func (s *SecurityContext) IsAuthenticated() bool {
	return s.user != nil
}

// GetUsername returns the username of the authenticated user
// It returns null if the user has not been authenticated
func (s *SecurityContext) GetUsername() string {
	if !s.IsAuthenticated() {
		return ""
	}
	return s.user.Username
}

// IsSysAdmin returns whether the authenticated user is system admin
// It returns false if the user has not been authenticated
func (s *SecurityContext) IsSysAdmin() bool {
	if !s.IsAuthenticated() {
		return false
	}
	return s.user.HasAdminRole
}

// IsSolutionUser ...
func (s *SecurityContext) IsSolutionUser() bool {
	return false
}

// HasReadPerm returns whether the user has read permission to the project
func (s *SecurityContext) HasReadPerm(projectIDOrName interface{}) bool {
	// public project
	public, err := s.pm.IsPublic(projectIDOrName)
	if err != nil {
		log.Errorf("failed to check the public of project %v: %v",
			projectIDOrName, err)
		return false
	}
	if public {
		return true
	}

	// private project
	if !s.IsAuthenticated() {
		return false
	}

	// system admin
	if s.IsSysAdmin() {
		return true
	}

	roles := s.GetProjectRoles(projectIDOrName)
	return len(roles) > 0
}

// HasWritePerm returns whether the user has write permission to the project
func (s *SecurityContext) HasWritePerm(projectIDOrName interface{}) bool {
	if !s.IsAuthenticated() {
		return false
	}
	// system admin
	if s.IsSysAdmin() {
		return true
	}
	roles := s.GetProjectRoles(projectIDOrName)
	for _, role := range roles {
		switch role {
		case common.RoleProjectAdmin,
			common.RoleDeveloper:
			return true
		}
	}
	return false
}

// HasAllPerm returns whether the user has all permissions to the project
func (s *SecurityContext) HasAllPerm(projectIDOrName interface{}) bool {
	if !s.IsAuthenticated() {
		return false
	}
	// system admin
	if s.IsSysAdmin() {
		return true
	}
	roles := s.GetProjectRoles(projectIDOrName)
	for _, role := range roles {
		switch role {
		case common.RoleProjectAdmin:
			return true
		}
	}
	return false
}

// GetProjectRoles ...
func (s *SecurityContext) GetProjectRoles(projectIDOrName interface{}) []int {
	if !s.IsAuthenticated() || projectIDOrName == nil {
		return []int{}
	}

	roles := []int{}
	user, err := dao.GetUser(models.User{
		Username: s.GetUsername(),
	})
	if err != nil {
		log.Errorf("failed to get user %s: %v", s.GetUsername(), err)
		return roles
	}
	if user == nil {
		log.Debugf("user %s not found", s.GetUsername())
		return roles
	}
	project, err := s.pm.Get(projectIDOrName)
	if err != nil {
		log.Errorf("failed to get project %v: %v", projectIDOrName, err)
		return roles
	}
	if project == nil {
		log.Errorf("project %v not found", projectIDOrName)
		return roles
	}
	roleList, err := dao.GetUserProjectRoles(user.UserID, project.ProjectID, common.UserMember)
	if err != nil {
		log.Errorf("failed to get roles of user %d to project %d: %v", user.UserID, project.ProjectID, err)
		return roles
	}
	for _, role := range roleList {
		switch role.RoleCode {
		case "MDRWS":
			roles = append(roles, common.RoleProjectAdmin)
		case "RWS":
			roles = append(roles, common.RoleDeveloper)
		case "RS":
			roles = append(roles, common.RoleGuest)
		}
	}
	if len(roles) != 0 {
		return roles
	}
	return s.GetRolesByGroup(projectIDOrName)
}

// GetRolesByGroup - Get the group role of current user to the project
func (s *SecurityContext) GetRolesByGroup(projectIDOrName interface{}) []int {
	var roles []int
	user := s.user
	project, err := s.pm.Get(projectIDOrName)
	//No user, group or project info
	if err != nil || project == nil || user == nil || len(user.GroupList) == 0 {
		return roles
	}
	//Get role by LDAP group
	groupDNConditions := group.GetGroupDNQueryCondition(user.GroupList)
	roles, err = dao.GetRolesByLDAPGroup(project.ProjectID, groupDNConditions)
	if err != nil {
		return nil
	}
	return roles
}

// GetMyProjects ...
func (s *SecurityContext) GetMyProjects() ([]*models.Project, error) {
	result, err := s.pm.List(
		&models.ProjectQueryParam{
			Member: &models.MemberQuery{
				Name:      s.GetUsername(),
				GroupList: s.user.GroupList,
			},
		})
	if err != nil {
		return nil, err
	}

	return result.Projects, nil
}
