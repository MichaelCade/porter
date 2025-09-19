package main

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// Use external template file
var templates = template.Must(template.ParseFiles("/app/simple_template.html"))

// Upload progress tracking
var uploadProgress struct {
	sync.Mutex
	Current int
	Total   int
	Status  string
}

// Handler for reporting upload progress
func uploadProgressHandler(w http.ResponseWriter, r *http.Request) {
	uploadProgress.Lock()
	defer uploadProgress.Unlock()

	// Calculate percentage
	var percentage int
	if uploadProgress.Total > 0 {
		percentage = (uploadProgress.Current * 100) / uploadProgress.Total
	} else {
		percentage = 0
	}

	// Create response
	response := struct {
		Current    int    `json:"current"`
		Total      int    `json:"total"`
		Percentage int    `json:"percentage"`
		Status     string `json:"status"`
	}{
		Current:    uploadProgress.Current,
		Total:      uploadProgress.Total,
		Percentage: percentage,
		Status:     uploadProgress.Status,
	}

	// Send JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type UIData struct {
	Message         string
	VMDKs           []string
	ConvertedFiles  []string
	QemuAvailable   bool
	AwsCliAvailable bool
	AzCliAvailable  bool
	DockerNotice    string

	AzureAccounts   []string
	AzureContainers []string
}

const extractDir = "/app/extracted"
const convertDir = "/app/converted"

// Find VMDKs in the extracted directory
func findExistingVMDKs() []string {
	files := findFilesWithExtension(extractDir, ".vmdk")
	// For display purposes, let's return nice paths relative to the extraction directory
	for i, file := range files {
		if filepath.IsAbs(file) && strings.HasPrefix(file, extractDir) {
			files[i] = file // Keep the full path for the application to use
		}
	}
	return files
}

// Find converted files in the converted directory
func findExistingConvertedFiles() []string {
	// Look for raw, vhd, vhdx and qcow2 files
	rawFiles := findFilesWithExtension(convertDir, ".raw")
	vhdFiles := findFilesWithExtension(convertDir, ".vhd")
	vhdxFiles := findFilesWithExtension(convertDir, ".vhdx")
	qcow2Files := findFilesWithExtension(convertDir, ".qcow2")
	allFiles := append(append(append(rawFiles, vhdFiles...), vhdxFiles...), qcow2Files...)

	// For display purposes, let's return nice paths relative to the conversion directory
	for i, file := range allFiles {
		if filepath.IsAbs(file) && strings.HasPrefix(file, convertDir) {
			allFiles[i] = file // Keep the full path for the application to use
		}
	}
	return allFiles
}

// Helper function to find files with a specific extension in a directory
func findFilesWithExtension(dir string, ext string) []string {
	var files []string

	// First, search for files in the regular directory
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if file has the desired extension
		if strings.HasSuffix(strings.ToLower(info.Name()), ext) {
			files = append(files, path)
		}

		return nil
	})

	// Also check Docker container paths if in Docker
	if _, err := os.Stat("/.dockerenv"); err == nil {
		// We're in Docker, so check volume mounts
		dockerPath := filepath.Join("/app", filepath.Base(dir))
		if dockerPath != dir { // Only if not already the same path
			filepath.Walk(dockerPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil // Skip errors
				}

				// Skip directories
				if info.IsDir() {
					return nil
				}

				// Check if file has the desired extension
				if strings.HasSuffix(strings.ToLower(info.Name()), ext) {
					files = append(files, path)
				}

				return nil
			})
		}
	}

	// Debug output with raw paths
	if len(files) > 0 {
		fmt.Printf("Found %d raw %s files (before deduplication):\n", len(files), ext)
		for i, file := range files {
			fmt.Printf("  Raw file %d: %s\n", i+1, file)
		}

		// Remove duplicates by basename
		uniqueFiles := make(map[string]bool)
		var dedupedFiles []string

		for _, file := range files {
			baseFileName := filepath.Base(file)
			if !uniqueFiles[baseFileName] {
				uniqueFiles[baseFileName] = true
				dedupedFiles = append(dedupedFiles, file)
			}
		}

		// Debug output after deduplication
		fmt.Printf("Found %d unique %s file(s) after deduplication:\n", len(dedupedFiles), ext)
		for i, file := range dedupedFiles {
			fmt.Printf("  File %d: %s\n", i+1, file)
		}

		return dedupedFiles
	}

	return files
}

func main() {
	// Ensure directories exist
	os.MkdirAll(extractDir, 0755)
	os.MkdirAll(convertDir, 0755)

	// Log any existing files found
	existingVMDKs := findExistingVMDKs()
	existingConverted := findExistingConvertedFiles()

	if len(existingVMDKs) > 0 {
		fmt.Printf("Found %d existing VMDK file(s) in %s\n", len(existingVMDKs), extractDir)
		for i, vmdk := range existingVMDKs {
			fmt.Printf("  Existing VMDK %d: %s\n", i+1, filepath.Base(vmdk))
		}
	}

	if len(existingConverted) > 0 {
		fmt.Printf("Found %d existing converted file(s) in %s\n", len(existingConverted), convertDir)
		for i, converted := range existingConverted {
			fmt.Printf("  Existing converted file %d: %s\n", i+1, filepath.Base(converted))
		}
	}

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/extract", extractHandler)
	http.HandleFunc("/convert", convertHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/azure/accounts", azureAccountsHandler)
	http.HandleFunc("/azure/containers", azureContainersHandler)
	http.HandleFunc("/aws/buckets", awsBucketsHandler)
	http.HandleFunc("/upload/progress", uploadProgressHandler)

	fmt.Println("🚀 Porter is running on http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}

// Handler to fetch Azure accounts
func azureAccountsHandler(w http.ResponseWriter, r *http.Request) {
	accounts := listAzureAccounts()
	if accounts == nil {
		fmt.Println("Warning: No Azure accounts found or error occurred")
		accounts = []string{}
	} else {
		fmt.Printf("Found %d Azure accounts\n", len(accounts))
		for i, acc := range accounts {
			fmt.Printf("  Account %d: %s\n", i+1, acc)
		}
	}
	json.NewEncoder(w).Encode(map[string][]string{"accounts": accounts})
}

// Handler to fetch AWS S3 buckets dynamically
func awsBucketsHandler(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("aws", "s3api", "list-buckets", "--query", "Buckets[].Name", "--output", "text")
	out, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, "Failed to list S3 buckets: "+err.Error(), http.StatusInternalServerError)
		return
	}
	buckets := strings.Fields(string(out))
	json.NewEncoder(w).Encode(map[string][]string{"buckets": buckets})
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	// Look for existing files
	existingVMDKs := findExistingVMDKs()
	existingConverted := findExistingConvertedFiles()

	// Get Azure accounts
	accounts := listAzureAccounts()

	// Prepare message based on what was found
	message := "Ready"
	if len(existingVMDKs) > 0 || len(existingConverted) > 0 {
		parts := []string{"Ready"}

		if len(existingVMDKs) > 0 {
			parts = append(parts, fmt.Sprintf("Found %d existing VMDK file(s)", len(existingVMDKs)))
		}

		if len(existingConverted) > 0 {
			parts = append(parts, fmt.Sprintf("Found %d existing converted file(s)", len(existingConverted)))
		}

		message = strings.Join(parts, ". ")
	}

	data := UIData{
		Message:         message,
		VMDKs:           existingVMDKs,
		ConvertedFiles:  existingConverted,
		QemuAvailable:   checkBinary("qemu-img"),
		AwsCliAvailable: checkBinary("aws"),
		AzCliAvailable:  checkBinary("az"),
		DockerNotice:    dockerNotice(),
		AzureAccounts:   accounts,
	}
	templates.Execute(w, data)
}

// Extract OVA → VMDKs
func extractHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Extract handler called with method:", r.Method)
	fmt.Println("Content-Type:", r.Header.Get("Content-Type"))

	// Verify we have a POST request with the correct content type
	if r.Method != http.MethodPost {
		errMsg := "Invalid request method. Expected POST."
		fmt.Println(errMsg)
		http.Error(w, errMsg, http.StatusMethodNotAllowed)
		return
	}

	if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		errMsg := "Invalid content type. Expected multipart/form-data."
		fmt.Println(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	// Debug all form field names
	err := r.ParseMultipartForm(32 << 20) // 32MB max memory
	if err != nil {
		errMsg := fmt.Sprintf("Error parsing form: %s", err.Error())
		fmt.Println(errMsg)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	if r.MultipartForm == nil || r.MultipartForm.File == nil {
		errMsg := "No files uploaded in the form"
		fmt.Println(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	// Log all available form fields to help debug
	fmt.Println("Available form fields:")
	for key := range r.MultipartForm.File {
		fmt.Printf("- %s\n", key)
	}

	// Try both field names (ova and ovaFile)
	var file multipart.File
	var handler *multipart.FileHeader

	// First try "ova" (which we confirmed is in the template)
	file, handler, err = r.FormFile("ova")
	if err != nil {
		fmt.Printf("Failed to get 'ova': %s, trying 'ovaFile' instead...\n", err.Error())

		// Try ovaFile as fallback
		file, handler, err = r.FormFile("ovaFile")
		if err != nil {
			errMsg := fmt.Sprintf("Error reading OVA (tried both 'ova' and 'ovaFile' fields): %s", err.Error())
			fmt.Println(errMsg)
			http.Error(w, errMsg, http.StatusInternalServerError)
			return
		}
	}
	defer file.Close()

	if !hasFreeSpace(extractDir, 10) {
		http.Error(w, "Not enough free disk space to extract OVA!", http.StatusInsufficientStorage)
		return
	}

	fmt.Printf("Extracting OVA file: %s (size: %d bytes)\n", handler.Filename, handler.Size)

	tr := tar.NewReader(file)
	var vmdks []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			errMsg := fmt.Sprintf("Error extracting OVA: %s", err.Error())
			fmt.Println(errMsg)
			http.Error(w, errMsg, http.StatusInternalServerError)
			return
		}

		target := filepath.Join(extractDir, hdr.Name)
		if hdr.FileInfo().IsDir() {
			os.MkdirAll(target, hdr.FileInfo().Mode())
		} else {
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.Create(target)
			if err != nil {
				errMsg := fmt.Sprintf("Error creating file %s: %s", target, err.Error())
				fmt.Println(errMsg)
				http.Error(w, errMsg, http.StatusInternalServerError)
				return
			}

			_, err = io.Copy(f, tr)
			f.Close()
			if err != nil {
				errMsg := fmt.Sprintf("Error writing to file %s: %s", target, err.Error())
				fmt.Println(errMsg)
				http.Error(w, errMsg, http.StatusInternalServerError)
				return
			}

			if strings.HasSuffix(hdr.Name, ".vmdk") {
				vmdks = append(vmdks, target)
				fmt.Printf("Extracted VMDK: %s\n", target)
			}
		}
	}

	fmt.Printf("OVA extraction completed. Found %d VMDKs\n", len(vmdks))

	statusMessage := fmt.Sprintf("Successfully extracted %d VMDK(s) from %s", len(vmdks), handler.Filename)
	data := UIData{
		Message:         statusMessage,
		VMDKs:           vmdks,
		QemuAvailable:   checkBinary("qemu-img"),
		AwsCliAvailable: checkBinary("aws"),
		AzCliAvailable:  checkBinary("az"),
		DockerNotice:    dockerNotice(),
		AzureAccounts:   listOrEmpty(listAzureAccounts()),
	}
	templates.Execute(w, data)
}

// Convert multiple VMDKs
func convertHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	format := r.FormValue("format")
	selectedFiles := r.Form["vmdks"]

	// Set default format to raw if not specified
	if format == "" {
		format = "raw"
	}

	// Validate format is supported
	supportedFormats := map[string]bool{
		"raw":   true,
		"vpc":   true,
		"qcow2": true,
		"vhdx":  true,
	}

	if !supportedFormats[format] {
		http.Error(w, "Unsupported conversion format: "+format, http.StatusBadRequest)
		return
	}

	if len(selectedFiles) == 0 {
		// Return to the main page with a friendly message instead of an error
		data := UIData{
			Message:         "No VMDK files selected for conversion. Please extract an OVA or select files to convert.",
			VMDKs:           findExistingVMDKs(),
			ConvertedFiles:  findExistingConvertedFiles(),
			QemuAvailable:   checkBinary("qemu-img"),
			AwsCliAvailable: checkBinary("aws"),
			AzCliAvailable:  checkBinary("az"),
			DockerNotice:    dockerNotice(),
			AzureAccounts:   listOrEmpty(listAzureAccounts()),
		}
		templates.Execute(w, data)
		return
	}

	if !hasFreeSpace(convertDir, 10) {
		http.Error(w, "Not enough free disk space to convert VMDKs!", http.StatusInsufficientStorage)
		return
	}

	fmt.Printf("Starting conversion of %d VMDK(s) to %s format\n", len(selectedFiles), format)

	var converted []string
	for i, input := range selectedFiles {
		fmt.Printf("[%d/%d] Converting %s to %s format\n", i+1, len(selectedFiles), input, format)

		// Determine the actual file extension users would expect based on the format
		var fileExtension string
		if format == "vpc" {
			fileExtension = "vhd" // Use VHD extension for VPC format
		} else if format == "vhdx" {
			fileExtension = "vhdx" // Use VHDX extension for VHDX format
		} else {
			fileExtension = format // For raw and other formats, use the format name directly
		}

		// Use qemu's internal format for the conversion command
		output := filepath.Join(convertDir, filepath.Base(input)+"."+fileExtension)
		os.MkdirAll(convertDir, 0755)

		cmd := exec.Command("qemu-img", "convert", "-f", "vmdk", "-O", format, input, output)
		out, err := cmd.CombinedOutput()
		if err != nil {
			errMsg := fmt.Sprintf("Conversion failed for %s: %s\nOutput: %s\n", input, err, string(out))
			fmt.Println(errMsg)
			http.Error(w, errMsg, http.StatusInternalServerError)
			return
		}

		// Get file size for reporting
		fileInfo, err := os.Stat(output)
		var fileSize int64
		if err == nil {
			fileSize = fileInfo.Size()
			fmt.Printf("Converted %s to %s (%.2f MB)\n", input, output, float64(fileSize)/(1024*1024))
		} else {
			fmt.Printf("Converted %s to %s (size unknown)\n", input, output)
		}

		converted = append(converted, output)
	}

	fmt.Printf("All conversions completed successfully\n")

	// Create a user-friendly format name for display
	formatDisplayName := format
	if format == "vpc" {
		formatDisplayName = "VHD (Hyper-V/Azure)"
	} else if format == "raw" {
		formatDisplayName = "RAW"
	} else if format == "qcow2" {
		formatDisplayName = "QCOW2 (QEMU/OpenStack)"
	} else if format == "vhdx" {
		formatDisplayName = "VHDX (Hyper-V)"
	}

	data := UIData{
		Message:         fmt.Sprintf("Successfully converted %d file(s) to %s format", len(converted), formatDisplayName),
		ConvertedFiles:  converted,
		VMDKs:           selectedFiles, // Keep the VMDK list so user can convert again if needed
		QemuAvailable:   checkBinary("qemu-img"),
		AwsCliAvailable: checkBinary("aws"),
		AzCliAvailable:  checkBinary("az"),
		DockerNotice:    dockerNotice(),
		AzureAccounts:   listOrEmpty(listAzureAccounts()),
	}
	templates.Execute(w, data)
}

// Upload to cloud/local
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	cloud := r.FormValue("cloud")
	files := r.Form["files"]
	target := r.FormValue("target")
	subscription := r.FormValue("account")
	containerFull := r.FormValue("container")
	bucket := r.FormValue("bucket")

	// Initialize progress tracking
	uploadProgress.Lock()
	uploadProgress.Current = 0
	uploadProgress.Total = len(files)
	uploadProgress.Status = "Starting upload..."
	uploadProgress.Unlock()

	fmt.Printf("Starting upload of %d file(s) to %s\n", len(files), cloud)

	// If no files selected, show a friendly error message in the UI rather than a plain HTTP error
	if len(files) == 0 {
		existingVMDKs := findExistingVMDKs()
		existingConverted := findExistingConvertedFiles()
		var message string

		if len(existingConverted) == 0 {
			message = "No files were found for upload. Please convert VMDKs first or place converted files in the conversion directory."
		} else {
			message = "Please select at least one file to upload."
		}

		data := UIData{
			Message:         message,
			VMDKs:           existingVMDKs,
			ConvertedFiles:  existingConverted,
			QemuAvailable:   checkBinary("qemu-img"),
			AwsCliAvailable: checkBinary("aws"),
			AzCliAvailable:  checkBinary("az"),
			DockerNotice:    dockerNotice(),
			AzureAccounts:   listOrEmpty(listAzureAccounts()),
		}
		templates.Execute(w, data)
		return
	}

	var message strings.Builder
	var successCount, failCount int

	for i, file := range files {
		switch cloud {
		case "aws":
			// Build S3 URI from selected bucket and optional target path
			s3Uri := "s3://" + bucket
			if target != "" {
				s3Uri += "/" + strings.TrimPrefix(target, "/")
			}
			s3Uri += "/" + filepath.Base(file)

			fmt.Printf("[%d/%d] Uploading %s to AWS S3: %s\n", i+1, len(files), file, s3Uri)

			// Get file size for progress reporting
			fileInfo, err := os.Stat(file)
			if err != nil {
				errMsg := fmt.Sprintf("Failed to get file info for %s: %s\n", file, err)
				fmt.Println(errMsg)
				message.WriteString(errMsg + "\n")
				failCount++
				continue
			}
			fileSize := fileInfo.Size()

			// Update progress tracking
			uploadProgress.Lock()
			uploadProgress.Current = i
			uploadProgress.Status = fmt.Sprintf("Uploading %s to AWS S3: %s (%.2f MB)",
				filepath.Base(file), s3Uri, float64(fileSize)/(1024*1024))
			uploadProgress.Unlock()

			// Use aws s3 cp with progress options
			cmd := exec.Command("aws", "s3", "cp", "--no-progress", file, s3Uri)

			// Create a pipe to capture stdout in real-time
			stdoutPipe, err := cmd.StdoutPipe()
			if err != nil {
				errMsg := fmt.Sprintf("Failed to create stdout pipe: %s\n", err)
				fmt.Println(errMsg)
				message.WriteString(errMsg + "\n")
				failCount++
				continue
			}

			// Create a pipe for stderr as well
			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				errMsg := fmt.Sprintf("Failed to create stderr pipe: %s\n", err)
				fmt.Println(errMsg)
				message.WriteString(errMsg + "\n")
				failCount++
				continue
			}

			// Start the command
			err = cmd.Start()
			if err != nil {
				errMsg := fmt.Sprintf("Failed to start AWS upload: %s\n", err)
				fmt.Println(errMsg)
				message.WriteString(errMsg + "\n")
				failCount++
				continue
			}

			// Print progress information during upload
			go func() {
				buffer := make([]byte, 1024)
				for {
					n, err := stdoutPipe.Read(buffer)
					if n > 0 {
						fmt.Print(string(buffer[:n]))
					}
					if err != nil {
						break
					}
				}
			}()

			go func() {
				buffer := make([]byte, 1024)
				for {
					n, err := stderrPipe.Read(buffer)
					if n > 0 {
						fmt.Print(string(buffer[:n]))
					}
					if err != nil {
						break
					}
				}
			}()

			// Wait for the command to complete
			err = cmd.Wait()
			if err != nil {
				errMsg := fmt.Sprintf("AWS upload failed for %s: %s\n", file, err)
				fmt.Println(errMsg)
				message.WriteString(errMsg + "\n")
				failCount++
				continue
			}

			successMsg := fmt.Sprintf("✅ AWS upload succeeded: %s to %s\n", file, s3Uri)
			fmt.Println(successMsg)
			message.WriteString(successMsg)
			successCount++

		case "azure":
			// Parse the storage account and container from the combined value
			parts := strings.Split(containerFull, "/")
			if len(parts) != 2 {
				errMsg := fmt.Sprintf("Invalid Azure container format '%s'. Expected 'storageAccount/container'\n", containerFull)
				fmt.Println(errMsg)
				message.WriteString(errMsg)
				failCount++
				continue
			}

			storageAccount := parts[0]
			container := parts[1]

			blobName := filepath.Base(file)
			if target != "" {
				blobName = strings.TrimPrefix(target, "/") + "/" + blobName
			}

			fmt.Printf("[%d/%d] Uploading %s to Azure: %s/%s/%s\n",
				i+1, len(files), file, storageAccount, container, blobName)

			// Get file size for progress reporting
			fileInfo, err := os.Stat(file)
			if err != nil {
				errMsg := fmt.Sprintf("Failed to get file info for %s: %s\n", file, err)
				fmt.Println(errMsg)
				message.WriteString(errMsg + "\n")
				failCount++
				continue
			}
			fileSize := fileInfo.Size()

			// Update progress tracking
			uploadProgress.Lock()
			uploadProgress.Current = i
			uploadProgress.Status = fmt.Sprintf("Uploading %s to Azure: %s/%s/%s (%.2f MB)",
				filepath.Base(file), storageAccount, container, blobName, float64(fileSize)/(1024*1024))
			uploadProgress.Unlock()

			// Use az storage blob upload for uploading
			cmd := exec.Command("az", "storage", "blob", "upload",
				"--subscription", subscription,
				"--account-name", storageAccount,
				"--container-name", container,
				"--auth-mode", "login",
				"--name", blobName,
				"--file", file)

			// Create a pipe to capture stdout in real-time
			stdoutPipe, err := cmd.StdoutPipe()
			if err != nil {
				errMsg := fmt.Sprintf("Failed to create stdout pipe: %s\n", err)
				fmt.Println(errMsg)
				message.WriteString(errMsg + "\n")
				failCount++
				continue
			}

			// Create a pipe for stderr as well
			stderrPipe, err := cmd.StderrPipe()
			if err != nil {
				errMsg := fmt.Sprintf("Failed to create stderr pipe: %s\n", err)
				fmt.Println(errMsg)
				message.WriteString(errMsg + "\n")
				failCount++
				continue
			}

			// Start the command
			err = cmd.Start()
			if err != nil {
				errMsg := fmt.Sprintf("Failed to start Azure upload: %s\n", err)
				fmt.Println(errMsg)
				message.WriteString(errMsg + "\n")
				failCount++
				continue
			}

			// Print progress information during upload
			go func() {
				buffer := make([]byte, 1024)
				for {
					n, err := stdoutPipe.Read(buffer)
					if n > 0 {
						fmt.Print(string(buffer[:n]))
					}
					if err != nil {
						break
					}
				}
			}()

			go func() {
				buffer := make([]byte, 1024)
				for {
					n, err := stderrPipe.Read(buffer)
					if n > 0 {
						fmt.Print(string(buffer[:n]))
					}
					if err != nil {
						break
					}
				}
			}()

			// Wait for the command to complete
			err = cmd.Wait()
			if err != nil {
				errMsg := fmt.Sprintf("Azure upload failed for %s: %s\n", file, err)
				fmt.Println(errMsg)
				message.WriteString(errMsg + "\n")
				failCount++
				continue
			}

			successMsg := fmt.Sprintf("✅ Azure upload succeeded: %s to %s/%s/%s\n",
				file, storageAccount, container, blobName)
			fmt.Println(successMsg)
			message.WriteString(successMsg)
			successCount++
		case "local":
			if target == "" {
				target = "/data"
			}

			fmt.Printf("[%d/%d] Saving %s to local filesystem: %s\n", i+1, len(files), file, target)

			// Update progress tracking
			uploadProgress.Lock()
			uploadProgress.Current = i
			uploadProgress.Status = fmt.Sprintf("Copying %s to local filesystem: %s",
				filepath.Base(file), target)
			uploadProgress.Unlock()

			os.MkdirAll(target, 0755)
			dst := filepath.Join(target, filepath.Base(file))
			err := copyFile(file, dst)
			if err != nil {
				errMsg := fmt.Sprintf("Local copy failed for %s: %s\n", file, err)
				fmt.Println(errMsg)
				message.WriteString(errMsg + "\n")
				failCount++
				continue
			}

			successMsg := fmt.Sprintf("✅ Saved locally: %s\n", dst)
			fmt.Println(successMsg)
			message.WriteString(successMsg)
			successCount++

		default:
			errMsg := fmt.Sprintf("Unknown cloud target for %s\n", file)
			fmt.Println(errMsg)
			message.WriteString(errMsg + "\n")
			failCount++
		}
	}

	// Create a summary message
	summaryMsg := fmt.Sprintf("Upload summary: %d successful, %d failed", successCount, failCount)
	fmt.Println(summaryMsg)

	// Update progress tracking to completed
	uploadProgress.Lock()
	uploadProgress.Current = uploadProgress.Total
	uploadProgress.Status = fmt.Sprintf("Upload completed: %d successful, %d failed", successCount, failCount)
	uploadProgress.Unlock()

	// Determine status message based on results
	var messagePrefix string
	if failCount == 0 {
		messagePrefix = "✅ All uploads completed successfully! "
	} else if successCount == 0 {
		messagePrefix = "❌ All uploads failed. "
	} else {
		messagePrefix = "⚠️ Some uploads completed, some failed. "
	}

	data := UIData{
		Message:         fmt.Sprintf("%s%s\n\n%s", messagePrefix, summaryMsg, message.String()),
		ConvertedFiles:  files,
		QemuAvailable:   checkBinary("qemu-img"),
		AwsCliAvailable: checkBinary("aws"),
		AzCliAvailable:  checkBinary("az"),
		DockerNotice:    dockerNotice(),
		AzureAccounts:   listOrEmpty(listAzureAccounts()),
	}
	templates.Execute(w, data)
}

// Handler to fetch containers dynamically
func azureContainersHandler(w http.ResponseWriter, r *http.Request) {
	subscription := r.URL.Query().Get("account")
	fmt.Printf("Looking for containers in subscription: '%s'\n", subscription)

	containers, err := listAzureContainers(subscription)
	if err != nil {
		fmt.Printf("Error listing containers for subscription '%s': %s\n", subscription, err)
		http.Error(w, "Failed to list containers: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Printf("Found %d containers in subscription '%s'\n", len(containers), subscription)
	for i, container := range containers {
		fmt.Printf("  Container %d: %s\n", i+1, container)
	}

	json.NewEncoder(w).Encode(map[string][]string{"containers": containers})
}

// Helpers
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Sync()
}

func checkBinary(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

func dockerNotice() string {
	notice := ""
	if _, err := os.Stat("/.dockerenv"); err == nil {
		notice = "⚠️ Running inside Docker.\n" +
			"- Make sure you mounted ~/.aws and ~/.azure for credentials.\n" +
			"- It's recommended to mount host directories for /app/extracted and /app/converted " +
			"to avoid filling Docker.raw with large files. Otherwise, large temporary files may consume a lot of disk space."
	} else {
		notice = "✅ Running locally. Ensure qemu-img, aws CLI, and az CLI are installed."
	}
	return notice
}

func hasFreeSpace(path string, neededGB int64) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return false
	}
	available := stat.Bavail * uint64(stat.Bsize)
	return available >= uint64(neededGB*1024*1024*1024)
}

func listAzureAccounts() []string {
	cmd := exec.Command("az", "account", "list", "--query", "[].name", "-o", "tsv")
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error listing Azure accounts: %s\nOutput: %s\n", err, string(out))
		// Return empty list instead of nil for better UI handling
		return []string{}
	}
	// Split by newlines instead of whitespace to preserve account names with spaces
	accounts := strings.Split(strings.TrimSpace(string(out)), "\n")
	// Filter out empty strings
	var result []string
	for _, acc := range accounts {
		if acc != "" {
			result = append(result, acc)
		}
	}

	// Log the accounts found
	if len(result) > 0 {
		fmt.Printf("Found %d Azure accounts\n", len(result))
	} else {
		fmt.Println("No Azure accounts found")
	}

	return result
}

// First list storage accounts in the subscription, then list containers in each storage account
func listAzureContainers(subscription string) ([]string, error) {
	// Step 1: List storage accounts in the subscription
	cmdAccounts := exec.Command("az", "storage", "account", "list",
		"--subscription", subscription,
		"--query", "[].name",
		"-o", "tsv")

	outAccounts, err := cmdAccounts.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list storage accounts: %w\nOutput: %s", err, outAccounts)
	}

	storageAccounts := strings.Split(strings.TrimSpace(string(outAccounts)), "\n")
	if len(storageAccounts) == 0 || (len(storageAccounts) == 1 && storageAccounts[0] == "") {
		return nil, fmt.Errorf("no storage accounts found in subscription '%s'", subscription)
	}

	fmt.Printf("Found %d storage accounts in subscription '%s'\n", len(storageAccounts), subscription)

	// Step 2: List containers for each storage account
	var allContainers []string
	for _, storageAccount := range storageAccounts {
		if storageAccount == "" {
			continue
		}

		fmt.Printf("Listing containers for storage account '%s'\n", storageAccount)
		cmdContainers := exec.Command("az", "storage", "container", "list",
			"--subscription", subscription,
			"--account-name", storageAccount,
			"--auth-mode", "login",
			"--query", "[].name",
			"-o", "tsv")

		outContainers, err := cmdContainers.CombinedOutput()
		if err != nil {
			fmt.Printf("Warning: Failed to list containers for storage account '%s': %v\n",
				storageAccount, err)
			continue // Try next storage account instead of failing completely
		}

		containers := strings.Split(strings.TrimSpace(string(outContainers)), "\n")
		for _, container := range containers {
			if container != "" {
				// Include storage account name with container for clarity
				containerWithAccount := fmt.Sprintf("%s/%s", storageAccount, container)
				allContainers = append(allContainers, containerWithAccount)
			}
		}
	}

	return allContainers, nil
}

func listOrEmpty(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}
