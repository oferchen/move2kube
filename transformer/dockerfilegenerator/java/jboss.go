/*
 *  Copyright IBM Corporation 2021
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package java

import (
	"os"
	"path/filepath"

	"github.com/konveyor/move2kube/common"
	"github.com/konveyor/move2kube/environment"
	irtypes "github.com/konveyor/move2kube/types/ir"
	transformertypes "github.com/konveyor/move2kube/types/transformer"
	"github.com/konveyor/move2kube/types/transformer/artifacts"
	"github.com/sirupsen/logrus"
)

var (
	jbossDefaultPort int32 = 9080
)

// Jboss implements Transformer interface
type Jboss struct {
	Config      transformertypes.Transformer
	Env         *environment.Environment
	JbossConfig *JbossYamlConfig
}

// JbossYamlConfig stores jar related configuration information
type JbossYamlConfig struct {
	JavaVersion string `yaml:"defaultJavaVersion"`
	DefaultPort int32  `yaml:"defaultPort"`
}

// JbossDockerfileTemplate stores parameters for the dockerfile template
type JbossDockerfileTemplate struct {
	JavaPackageName                   string
	DeploymentFile                    string
	BuildContainerName                string
	DeploymentFileDirInBuildContainer string
	Port                              int32
	EnvVariables                      map[string]string
}

// Init Initializes the transformer
func (t *Jboss) Init(tc transformertypes.Transformer, env *environment.Environment) (err error) {
	t.Config = tc
	t.Env = env
	t.JbossConfig = &JbossYamlConfig{}
	err = common.GetObjFromInterface(t.Config.Spec.Config, &t.JbossConfig)
	if err != nil {
		logrus.Errorf("unable to load config for Transformer %+v into %T : %s", t.Config.Spec.Config, t.JbossConfig, err)
		return err
	}
	return nil
}

// GetConfig returns the transformer config
func (t *Jboss) GetConfig() (transformertypes.Transformer, *environment.Environment) {
	return t.Config, t.Env
}

// DirectoryDetect runs detect in each sub directory
func (t *Jboss) DirectoryDetect(dir string) (services map[string][]transformertypes.Artifact, err error) {
	return
}

// Transform transforms the artifacts
func (t *Jboss) Transform(newArtifacts []transformertypes.Artifact, alreadySeenArtifacts []transformertypes.Artifact) ([]transformertypes.PathMapping, []transformertypes.Artifact, error) {
	pathMappings := []transformertypes.PathMapping{}
	createdArtifacts := []transformertypes.Artifact{}
	for _, a := range newArtifacts {
		var sConfig artifacts.ServiceConfig
		err := a.GetConfig(artifacts.ServiceConfigType, &sConfig)
		if err != nil {
			logrus.Errorf("unable to load config for Transformer into %T : %s", sConfig, err)
			continue
		}
		sImageName := artifacts.ImageName{}
		err = a.GetConfig(artifacts.ImageNameConfigType, &sImageName)
		if err != nil {
			logrus.Debugf("unable to load config for Transformer into %T : %s", sImageName, err)
		}
		if sImageName.ImageName == "" {
			sImageName.ImageName = common.MakeStringContainerImageNameCompliant(sConfig.ServiceName)
		}
		relSrcPath, err := filepath.Rel(t.Env.GetEnvironmentSource(), a.Paths[artifacts.ProjectPathPathType][0])
		if err != nil {
			logrus.Errorf("Unable to convert source path %s to be relative : %s", a.Paths[artifacts.ProjectPathPathType][0], err)
		}
		jbossRunDockerfile, err := os.ReadFile(filepath.Join(t.Env.GetEnvironmentContext(), t.Env.RelTemplatesDir, "Dockerfile.jboss"))
		if err != nil {
			logrus.Errorf("Unable to read Dockerfile jboss template : %s", err)
		}
		dockerFileHead := ""
		isBuildContainerPresent := false
		if buildContainerPaths, ok := a.Paths[artifacts.BuildContainerFileType]; ok && len(buildContainerPaths) > 0 {
			isBuildContainerPresent = true
			dockerfileBuildDockerfile := buildContainerPaths[0]
			dockerFileHeadBytes, err := os.ReadFile(dockerfileBuildDockerfile)
			if err != nil {
				logrus.Errorf("Unable to read build Dockerfile template : %s", err)
				continue
			}
			dockerFileHead = string(dockerFileHeadBytes)
		} else {
			dockerFileHeadBytes, err := os.ReadFile(filepath.Join(t.Env.GetEnvironmentContext(), t.Env.RelTemplatesDir, "Dockerfile.license"))
			if err != nil {
				logrus.Errorf("Unable to read Dockerfile license template : %s", err)
			}
			dockerFileHead = string(dockerFileHeadBytes)
		}
		tempDir := filepath.Join(t.Env.TempPath, a.Name)
		os.MkdirAll(tempDir, common.DefaultDirectoryPermission)
		dockerfileTemplate := filepath.Join(tempDir, common.DefaultDockerfileName)
		template := string(dockerFileHead) + "\n" + string(jbossRunDockerfile)
		err = os.WriteFile(dockerfileTemplate, []byte(template), common.DefaultFilePermission)
		if err != nil {
			logrus.Errorf("Could not write the generated Build Dockerfile template: %s", err)
		}
		dft := JbossDockerfileTemplate{}
		jbossArtifactConfig := artifacts.WarArtifactConfig{}
		err = a.GetConfig(artifacts.WarConfigType, &jbossArtifactConfig)
		if err != nil {
			logrus.Debugf("unable to load config for Transformer into %T : %s", jbossArtifactConfig, err)
			jbossEarArtifactConfig := artifacts.EarArtifactConfig{}
			err = a.GetConfig(artifacts.EarConfigType, &jbossEarArtifactConfig)
			if err != nil {
				logrus.Debugf("unable to load config for Transformer into %T : %s", jbossEarArtifactConfig, err)
			}
			javaPackage, err := getJavaPackage(filepath.Join(t.Env.GetEnvironmentContext(), versionMappingFilePath), jbossEarArtifactConfig.JavaVersion)
			if err != nil {
				logrus.Errorf("Unable to find mapping version for java version %s : %s", jbossEarArtifactConfig.JavaVersion, err)
				javaPackage = "java-1.8.0-openjdk-devel"
			}
			if isBuildContainerPresent {
				dft.JavaPackageName = javaPackage
				dft.DeploymentFile = jbossEarArtifactConfig.DeploymentFile
				dft.BuildContainerName = jbossEarArtifactConfig.BuildContainerName
				dft.DeploymentFileDirInBuildContainer = jbossEarArtifactConfig.DeploymentFileDirInBuildContainer
				dft.Port = jbossDefaultPort
				dft.EnvVariables = jbossEarArtifactConfig.EnvVariables
			} else {
				dft.JavaPackageName = javaPackage
				dft.DeploymentFile = jbossEarArtifactConfig.DeploymentFile
				dft.Port = jbossDefaultPort
				dft.EnvVariables = jbossEarArtifactConfig.EnvVariables
			}
		} else {
			javaPackage, err := getJavaPackage(filepath.Join(t.Env.GetEnvironmentContext(), versionMappingFilePath), jbossArtifactConfig.JavaVersion)
			if err != nil {
				logrus.Errorf("Unable to find mapping version for java version %s : %s", jbossArtifactConfig.JavaVersion, err)
				javaPackage = "java-1.8.0-openjdk-devel"
			}
			if isBuildContainerPresent {
				dft.JavaPackageName = javaPackage
				dft.DeploymentFile = jbossArtifactConfig.DeploymentFile
				dft.BuildContainerName = jbossArtifactConfig.BuildContainerName
				dft.DeploymentFileDirInBuildContainer = jbossArtifactConfig.DeploymentFileDirInBuildContainer
				dft.Port = jbossDefaultPort
				dft.EnvVariables = jbossArtifactConfig.EnvVariables
			} else {
				dft.JavaPackageName = javaPackage
				dft.DeploymentFile = jbossArtifactConfig.DeploymentFile
				dft.Port = jbossDefaultPort
				dft.EnvVariables = jbossArtifactConfig.EnvVariables
			}
		}
		pathMappings = append(pathMappings, transformertypes.PathMapping{
			Type:     transformertypes.SourcePathMappingType,
			DestPath: common.DefaultSourceDir,
		})
		if t.JbossConfig.DefaultPort != 0 {
			jbossDefaultPort = t.JbossConfig.DefaultPort
		}
		pathMappings = append(pathMappings, transformertypes.PathMapping{
			Type:           transformertypes.TemplatePathMappingType,
			SrcPath:        dockerfileTemplate,
			DestPath:       filepath.Join(common.DefaultSourceDir, relSrcPath),
			TemplateConfig: dft,
		})
		paths := a.Paths
		paths[artifacts.DockerfilePathType] = []string{filepath.Join(common.DefaultSourceDir, relSrcPath, common.DefaultDockerfileName)}
		p := transformertypes.Artifact{
			Name:     sImageName.ImageName,
			Artifact: artifacts.DockerfileArtifactType,
			Paths:    paths,
			Configs: map[string]interface{}{
				artifacts.ImageNameConfigType: sImageName,
			},
		}
		dfs := transformertypes.Artifact{
			Name:     sConfig.ServiceName,
			Artifact: artifacts.DockerfileForServiceArtifactType,
			Paths:    a.Paths,
			Configs: map[string]interface{}{
				artifacts.ImageNameConfigType: sImageName,
				artifacts.ServiceConfigType:   sConfig,
			},
		}
		ir := irtypes.IR{}
		if err = a.GetConfig(irtypes.IRConfigType, &ir); err == nil {
			dfs.Configs[irtypes.IRConfigType] = ir
		}
		createdArtifacts = append(createdArtifacts, p, dfs)
	}
	return pathMappings, createdArtifacts, nil
}