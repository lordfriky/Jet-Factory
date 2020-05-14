package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
)

type (
	// Distribution : Represent a distribution conatining a name, version, desktop environment and an optional list of packages
	Distribution struct {
		Name          string              `json:"name"`
		Configs       []string            `json:"configs"`
		Packages      []string            `json:"packages"`
		Architectures map[string][]string `json:"buildarch"`
		Variants      []Variant           `json:"variants"`
	}

	// Variant : Represent a distribution variant
	Variant struct {
		Name     string   `json:"name"`
		Configs  []string `json:"configs"`
		Packages []string `json:"packages"`
	}
)

var (
	distribution              Distribution
	variant                   Variant
	baseName, buildarch       string
	imageFile, packageManager string
	isVariant, isAndroid      = false, false
	hekate, staging           bool
	prepare, configs          bool
	packages, image           bool

	managerList = []string{"zypper", "dnf", "yum", "pacman", "apt"}

	dockerImageName = "docker.io/library/ubuntu:18.04"
	baseJSON, _     = ioutil.ReadFile("./base.json")
	basesDistro     = []Distribution{}
	_               = json.Unmarshal([]byte(baseJSON), &basesDistro)

	hekateVersion = "5.2.0"
	nyxVersion    = "0.9.0"
	hekateBin     = "hekate_ctcaer_" + hekateVersion + ".bin"
	hekateURL     = "https://github.com/CTCaer/hekate/releases/download/v" + hekateVersion + "/hekate_ctcaer_" + hekateVersion + "_Nyx_" + nyxVersion + ".zip"
	hekateZip     = hekateURL[strings.LastIndex(hekateURL, "/")+1:]
)

// DetectPackageManager :
func DetectPackageManager() (err error) {
	for _, man := range managerList {
		if _, err := os.Stat("/usr/bin/" + man); os.IsExist(err) {
			packageManager = man
			return nil
		}
	}
	return err
}

/* Rootfs Image creation
* Chroot into the filesystem
 */

// IsDistro : Checks if a distribution is avalaible in the config files
func IsDistro(name string) (err error) {
	// Check if name match a known distribution
	for i := 0; i < len(basesDistro); i++ {
		if name == basesDistro[i].Name {
			baseName = basesDistro[i].Name
			distribution = Distribution{Name: basesDistro[i].Name, Architectures: basesDistro[i].Architectures, Configs: basesDistro[i].Configs, Packages: basesDistro[i].Packages}
			return nil
		}
		for j := 0; j < len(basesDistro[i].Variants); j++ {
			if name == basesDistro[i].Variants[j].Name {
				isVariant = true
				variant = Variant{Name: basesDistro[i].Variants[j].Name}
				return nil
			}
		}
	}
	return err
}

// IsValidArchitecture : Check if the inputed architecture can be found for the distribution
func IsValidArchitecture() (archi *string) {
	for archis := range distribution.Architectures {
		if buildarch == archis {
			return &buildarch
		}
	}
	return nil
}

// PrepareFiles :
func PrepareFiles(basePath string) (err error) {
	if err = os.MkdirAll(basePath+"/tmp/", os.ModePerm); err != nil {
		return err
	}

	if err = os.MkdirAll(basePath+"/disk/", os.ModePerm); err != nil {
		return err
	}

	if err = os.MkdirAll("./downloadedFiles/", os.ModeDir); err != nil {
		return err
	}

	if hekate {
		if _, err := os.Stat("./downloadedFiles/" + hekateZip); os.IsNotExist(err) {
			fmt.Println("Downloading:", hekateZip)
			if err := Wget(hekateURL, "./downloadedFiles/"+hekateZip); err != nil {
				return err
			}
		}
	}

	image, err := DownloadURLfromTags("./downloadedFiles")
	if err != nil {
		return err
	}

	if strings.Contains("./downloadedFiles/"+image, ".raw") {
		if _, err := os.Stat("./downloadedFiles/" + image[0:strings.LastIndex(image, ".")]); os.IsNotExist(err) {
			if err := ExtractFiles("./downloadedFiles/"+image, "./downloadedFiles/"); err != nil {
				return err
			}
		}

		image = image[0:strings.LastIndex(image, ".")]
		if _, err := MountImage("./downloadedFiles/"+image, basePath+"/tmp/"); err != nil {
			return err
		}

		if _, err := DiskCopy(basePath+"/tmp/*", basePath); err != nil {
			return err
		}

		if _, err := Unmount(basePath + "/tmp/"); err != nil {
			return err
		}
	} else {
		if err := ExtractFiles("./downloadedFiles/"+image, basePath+"/tmp"); err != nil {
			return err
		}
	}
	return nil
}

// DownloadURLfromTags : Retrieve a URL for a distribution based on a version
func DownloadURLfromTags(dst string) (image string, err error) {
	var constructedURL string
	var versions, images []string

	for _, avalaibleMirror := range distribution.Architectures[buildarch] {
		if strings.Contains(avalaibleMirror, "{VERSION}") {
			constructedURL = strings.Split(avalaibleMirror, "/{VERSION}")[0]
			versionBody := WalkURL(constructedURL)

			search, _ := regexp.Compile(">:?([[:digit:]]{1,3}.[[:digit:]]+|[[:digit:]]+)(?:/)")
			match := search.FindAllStringSubmatch(*versionBody, -1)

			for i := 0; i < len(match); i++ {
				for _, submatches := range match {
					versions = append(versions, submatches[1])
				}
			}

			version, err := CliSelector("Select a version: ", versions)
			if err != nil {
				return "", err
			}

			constructedURL = strings.Replace(avalaibleMirror, "{VERSION}", version, 1)
			imageBody := WalkURL(constructedURL)

			search, _ = regexp.Compile(">:?([[:alpha:]]+.*.raw.xz)")
			imageMatch := search.FindAllStringSubmatch(*imageBody, -1)

			for i := 0; i < len(imageMatch); i++ {
				for _, submatches := range imageMatch {
					images = append(images, submatches[1])
				}
			}

			if len(images) > 1 {
				imageFile, err = CliSelector("Select an image file: ", images)
				if err != nil {
					return "", err
				}
			} else {
				imageFile = images[0]
			}

			imageFile = strings.TrimSpace(imageFile)
			constructedURL = constructedURL + imageFile
			image = imageFile

		} else {
			constructedURL = avalaibleMirror
		}

		if _, err := url.ParseRequestURI(constructedURL); err != nil {
			fmt.Println("Couldn't found mirror:", constructedURL)
			return "", err
		}

		if _, err := os.Stat(dst + "/" + image); os.IsNotExist(err) {
			fmt.Println("Mirror URL selected:", constructedURL)
			fmt.Println("Downloading:", image, "in:", dst)
			if err := Wget(constructedURL, dst+"/"+image); err != nil {
				return "", err
			}
		}
	}
	return image, nil
}

// ApplyConfigsInChrootEnv : Runs one or multiple command in a chroot environment; Returns nil if successful
func ApplyConfigsInChrootEnv(path [2]string) error {
	if err := PreChroot(path); err != nil {
		return err
	}

	if isVariant {
		for _, config := range variant.Configs {
			if err := SpawnContainer([]string{"arch-chroot", config, path[1]}, nil, path); err != nil {
				return err
			}
		}
	}

	for _, config := range distribution.Configs {
		if err := SpawnContainer([]string{"arch-chroot", config, path[1]}, nil, path); err != nil {
			return err
		}
	}

	if err := PostChroot(path); err != nil {
		return err
	}

	return nil
}

// InstallPackagesInChrootEnv : Installs packages list; Returns nil if successful
func InstallPackagesInChrootEnv(path [2]string) error {
	if err := PreChroot(path); err != nil {
		return err
	}

	// TODO-3 : Handle staging packages
	if isVariant {
		if err := SpawnContainer([]string{"arch-chroot", "`/bin/bash /tools/findPackageManager.sh`", strings.Join(variant.Packages, ","), path[1]}, nil, path); err != nil {
			return err
		}
	}

	if err := SpawnContainer([]string{"arch-chroot", "`/bin/bash /tools/findPackageManager.sh`", strings.Join(distribution.Packages, ","), path[1]}, nil, path); err != nil {
		return err
	}

	if err := PostChroot(path); err != nil {
		return err
	}

	return nil
}

// Factory : Build your distribution with the setted options; Returns a pointer on the location of the produced build
func Factory(distro string, dst string) (err error) {
	basePath := dst + "/" + distro

	if err := IsDistro(distro); err != nil {
		flag.Usage()
		return err
	}

	if !isAndroid {
		fmt.Println("Building:", distro, "\nInside directory:", basePath)
		path := [2]string{basePath, "/root/" + distro}

		if archi := IsValidArchitecture(); archi == nil {
			fmt.Println(buildarch, "is not a valid architecture !")
			return err
		}
		fmt.Println("Found valid architecture: ", buildarch)

		if prepare {
			if err := PrepareFiles(basePath); err != nil {
				return err
			}
		}

		if configs {
			if err := InstallPackagesInChrootEnv(path); err != nil {
				return err
			}
		}

		if packages {
			if err := ApplyConfigsInChrootEnv(path); err != nil {
				return err
			}
		}

		if image {

			if isVariant {
				CreateDisk(variant.Name+".img", basePath, "ext4")
				if _, err := MountImage(variant.Name+".img", basePath+"/disk"); err != nil {
					return err
				}
			} else {
				CreateDisk(baseName+".img", basePath, "ext4")
				if _, err := MountImage(baseName+".img", basePath+"/disk"); err != nil {
					return err
				}
			}

			if _, err := DiskCopy(basePath+"/*", basePath+"/disk/"); err != nil {
				return err
			}

			if hekate {
				// TODO - 4 : Implement split function and 7z compression
				if _, err := DiskCopy(basePath+"/boot/*", basePath); err != nil {
					return err
				}

			}

			if _, err := Unmount(basePath + "/disk/"); err != nil {
				return err
			}
		}
	} else {
		path := [2]string{basePath, "/root/android"}
		dockerImageName = "pablozaiden/switchroot-android-build:1.0.0"
		if err := SpawnContainer(nil, []string{"ROM_NAME=" + distro}, path); err != nil {
			return err
		}
	}
	fmt.Println("Done!")
	return nil
}

func main() {
	var distro, basepath string
	flag.StringVar(&distro, "distro", "", "the distro you want to build: ubuntu, fedora, gentoo, arch(blackarch, arch-bang), lineage(icosa, foster, foster_tab)")
	flag.StringVar(&basepath, "basepath", ".", "Path to use as Docker storage, can be a mounted external device")
	flag.StringVar(&buildarch, "arch", "aarch64", "Set the platform build architecture.")
	flag.BoolVar(&hekate, "hekate", false, "Build an hekate installable filesystem")
	flag.BoolVar(&staging, "staging", false, "Install built local packages")
	flag.BoolVar(&prepare, "prepare", false, "Build an hekate installable filesystem")
	flag.BoolVar(&configs, "configs", false, "Install built local packages")
	flag.BoolVar(&packages, "packages", false, "Build an hekate installable filesystem")
	flag.BoolVar(&image, "image", false, "Install built local packages")
	flag.Parse()

	// Sets default for android build
	if distro == "lineage" {
		distro = "icosa"
		isAndroid = true
	}

	// Sets default for opensuse build
	if distro == "opensuse" {
		distro = "leap"
	}

	log.Println(prepare)

	if distro == "" {
		flag.Usage()
	} else {
		Factory(distro, basepath)
	}
}
