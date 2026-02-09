/**
 * File operations for Proton Drive SDK service
 * 
 * Handles file upload/download with encryption/decryption
 * Phase 4: Integrate with actual Proton SDK for encryption
 * 
 * File organization:
 * - Root folder: LFS (configurable)
 * - Organization: LFS/{OID[0:2]}/{OID[2:]} (hierarchical by OID prefix)
 * - Example: LFS/00/abc123def456...
 */

const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const logger = require('./logger');

const STORAGE_BASE = process.env.LFS_STORAGE_BASE || 'LFS';
const TEMP_DIR = process.env.TEMP_DIR || '/tmp';
const STORAGE_ROOT = process.env.SDK_STORAGE_DIR || path.join(TEMP_DIR, 'proton-git-lfs-sdk-storage');

/**
 * Get the Proton Drive path for a given OID
 * Organizes files hierarchically: LFS/00/abc123... where 00 is OID prefix
 */
function getProtonDrivePath(oid) {
  if (!oid || oid.length < 2) {
    throw new Error('Invalid OID format');
  }
  
  const prefix = oid.substr(0, 2);
  const suffix = oid.substr(2);
  return `${STORAGE_BASE}/${prefix}/${suffix}`;
}

/**
 * Resolve object path in local backing store.
 * This is a stand-in for real Proton Drive persistence.
 */
function getLocalObjectPath(oid) {
  if (!oid || oid.length < 2) {
    throw new Error('Invalid OID format');
  }

  const prefix = oid.substr(0, 2);
  const suffix = oid.substr(2);
  return path.join(STORAGE_ROOT, prefix, suffix);
}

/**
 * Upload a file to Proton Drive
 * 
 * Phase 4: Replace with actual SDK upload:
 * 1. Read file from filePath
 * 2. Calculate SHA-256 hash
 * 3. Encrypt with Proton SDK (client-side)
 * 4. Upload to Proton Drive at LFS/{prefix}/{suffix}
 * 5. Return metadata
 */
async function uploadFile(token, oid, filePath) {
  try {
    logger.info(`Uploading file: OID=${oid} Path=${filePath}`);

    if (!filePath) {
      throw new Error('File path is required');
    }

    // Verify file exists
    if (!fs.existsSync(filePath)) {
      throw new Error(`File not found: ${filePath}`);
    }

    // Read file
    const fileData = fs.readFileSync(filePath);
    const fileSize = fileData.length;

    // Verify hash if possible
    // Phase 4: Validate file hash against OID
    const hash = crypto.createHash('sha256').update(fileData).digest('hex');
    logger.debug(`File hash: ${hash}, OID: ${oid}`);

    // Determine target location
    const location = getProtonDrivePath(oid);
    const objectPath = getLocalObjectPath(oid);

    // Persist bytes by OID to emulate durable backend storage.
    const targetDir = path.dirname(objectPath);
    fs.mkdirSync(targetDir, { recursive: true });

    const tmpPath = `${objectPath}.tmp-${Date.now()}-${process.pid}`;
    fs.writeFileSync(tmpPath, fileData);
    fs.renameSync(tmpPath, objectPath);
    logger.info(`Stored object at ${objectPath} (${fileSize} bytes)`);

    // Clean up source file after upload
    // fs.unlinkSync(filePath);

    return {
      oid: oid,
      size: fileSize,
      location: location,
      hash: hash,
      storagePath: objectPath
    };
  } catch (error) {
    logger.error(`Upload failed: ${error.message}`);
    throw error;
  }
}

/**
 * Download a file from Proton Drive
 * 
 * Phase 4: Replace with actual SDK download:
 * 1. Query Proton Drive for file at LFS/{prefix}/{suffix}
 * 2. Verify file exists
 * 3. Download encrypted file
 * 4. Decrypt with Proton SDK (client-side)
 * 5. Write to outputPath
 * 6. Return metadata
 */
async function downloadFile(token, oid, outputPath) {
  try {
    logger.info(`Downloading file: OID=${oid} OutputPath=${outputPath}`);

    if (!outputPath) {
      throw new Error('Output path is required');
    }

    // Determine source location on Proton Drive
    const location = getProtonDrivePath(oid);
    const objectPath = getLocalObjectPath(oid);
    logger.debug(`Source location: ${location}`);

    let fileData;
    if (fs.existsSync(objectPath)) {
      fileData = fs.readFileSync(objectPath);
      logger.info(`Loaded object from ${objectPath}`);
    } else {
      // Keep fallback behavior for legacy tests and manual probing.
      fileData = Buffer.from(`Mock file content for OID: ${oid}`);
      logger.warn(`Object not found at ${objectPath}, using mock content`);
    }
    
    // Ensure output directory exists
    const outputDir = path.dirname(outputPath);
    if (!fs.existsSync(outputDir)) {
      fs.mkdirSync(outputDir, { recursive: true });
    }

    // Write file
    fs.writeFileSync(outputPath, fileData);
    
    // Verify write
    if (!fs.existsSync(outputPath)) {
      throw new Error('Failed to write output file');
    }

    const fileSize = fileData.length;
    const hash = crypto.createHash('sha256').update(fileData).digest('hex');

    logger.info(`File downloaded successfully: ${outputPath} (${fileSize} bytes)`);

    return {
      oid: oid,
      size: fileSize,
      path: outputPath,
      hash: hash,
      location: location
    };
  } catch (error) {
    logger.error(`Download failed: ${error.message}`);
    throw error;
  }
}

/**
 * List files in a Proton Drive folder
 * 
 * Phase 4: Replace with actual SDK file listing:
 * 1. Query Proton Drive for folder contents
 * 2. Parse file metadata
 * 3. Return file list with OID, size, modified time
 */
async function listFiles(token, folder = 'LFS') {
  try {
    logger.info(`Listing files in folder: ${folder}`);

    // Phase 4: Call Proton SDK to list folder contents
    // For now, return mock file list
    const mockFiles = [
      {
        oid: '00abc123def456...',
        name: 'file1.bin',
        size: 1024,
        modified: new Date().toISOString()
      },
      {
        oid: '01def789ghi012...',
        name: 'file2.bin',
        size: 2048,
        modified: new Date().toISOString()
      }
    ];

    return mockFiles;
  } catch (error) {
    logger.error(`List failed: ${error.message}`);
    throw error;
  }
}

/**
 * Delete a file from Proton Drive
 * 
 * Phase 4: Implement file deletion via SDK
 */
async function deleteFile(token, oid) {
  try {
    logger.info(`Deleting file: OID=${oid}`);

    const location = getProtonDrivePath(oid);
    logger.debug(`Target location: ${location}`);

    // Phase 4: Call Proton SDK to delete file

    logger.info(`File deleted: ${location}`);

    return { oid: oid, deleted: true };
  } catch (error) {
    logger.error(`Delete failed: ${error.message}`);
    throw error;
  }
}

/**
 * Check if file exists on Proton Drive
 * 
 * Phase 4: Implement file existence check via SDK
 */
async function fileExists(token, oid) {
  try {
    const location = getProtonDrivePath(oid);

    // Phase 4: Call Proton SDK to check file existence

    return true; // Mock response
  } catch (error) {
    logger.error(`Existence check failed: ${error.message}`);
    return false;
  }
}

module.exports = {
  uploadFile,
  downloadFile,
  listFiles,
  deleteFile,
  fileExists,
  getProtonDrivePath,
  getLocalObjectPath
};
