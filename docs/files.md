# File Manager

The file manager provides a web interface for browsing, editing, and managing files within a domain's home directory.

## Navigation

Open the file manager from the domain detail page (**Files** tab). The interface shows:

- Directory tree on the left
- File listing on the right (name, size, type, permissions, last modified)
- Breadcrumb navigation

File types are displayed as `folder`, `file`, or `symlink`.

## Operations

### Browse and Read

Click a folder to enter it. Click a text file to view its contents inline with syntax highlighting.

### Upload

Drag and drop files or use the upload button. Multiple files and folders are supported. Uploads are chunked for large files.

### Create

| Action     | How                                               |
|------------|---------------------------------------------------|
| New file   | Right-click or use the toolbar, enter filename    |
| New folder | Right-click or use the toolbar, enter folder name |

### Edit

Open a text file and click **Edit**. The editor supports syntax highlighting for common formats. Save writes the file back with the same ownership and permissions.

### Rename / Move

Right-click a file or folder and choose **Rename** (rename in place) or **Move** (move to a different directory).

### Copy

Copy files within the domain's home directory. Select the file, then choose **Copy** and pick a destination.

### Delete

Delete files or folders. Folder deletion is recursive. Deleted files cannot be recovered through the panel — use backups if needed.

### Archive and Extract

| Operation | Supported formats                 |
|-----------|-----------------------------------|
| Archive   | `.zip`, `.tar`, `.tar.gz`         |
| Extract   | `.zip`, `.tar`, `.tar.gz`, `.rar` |

Select files and folders, then choose **Archive** to create a compressed archive. Upload or select an archive and choose **Extract** to unpack it. Extraction performs path traversal checks for security.

### Permissions (chmod)

Change file or folder permissions using the standard octal notation (e.g. `755`, `644`). The change applies recursively for folders.

### Calculate Size

Select a folder and choose **Calculate Size** to run `du` and get the total size of all contents.

### Search

Search for files by name within the current directory tree. Results show matching paths and sizes.

### Download

Click a file to download it through the browser. For folders, create an archive first.

## Security

- All file operations run as the domain's system user (not root)
- Paths are validated to prevent traversal outside the domain's home directory
- File types are validated during extraction to block symlink attacks
- Symlinks are detected but not followed during recursive operations

## Permissions and Ownership

The file manager respects the domain's POSIX ACL isolation:

- Each domain user can only access their own `/home/c_<user>/` directory
- The panel applies per-user ACL grants at startup via `HealHomePerms`
- SELinux contexts are preserved on file writes
