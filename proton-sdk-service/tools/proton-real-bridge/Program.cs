using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;
using Proton.Drive.Sdk;
using Proton.Drive.Sdk.Nodes;
using Proton.Drive.Sdk.Nodes.Download;
using Proton.Drive.Sdk.Nodes.Upload;
using Proton.Sdk;
using Proton.Sdk.Authentication;

return await ProtonRealBridge.RunAsync(args).ConfigureAwait(false);

internal static class ProtonRealBridge
{
    private static readonly Regex OidPattern = new("^[a-f0-9]{64}$", RegexOptions.Compiled | RegexOptions.CultureInvariant);

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true,
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        WriteIndented = false
    };

    public static async Task<int> RunAsync(string[] args)
    {
        var command = args.Length > 0 ? args[0].Trim().ToLowerInvariant() : string.Empty;
        if (string.IsNullOrWhiteSpace(command))
        {
            return await FailAsync(400, "missing bridge command").ConfigureAwait(false);
        }

        try
        {
            return command switch
            {
                "auth" => await HandleAuthAsync().ConfigureAwait(false),
                "upload" => await HandleUploadAsync().ConfigureAwait(false),
                "download" => await HandleDownloadAsync().ConfigureAwait(false),
                "list" => await HandleListAsync().ConfigureAwait(false),
                _ => await FailAsync(400, $"unsupported bridge command: {command}").ConfigureAwait(false),
            };
        }
        catch (BridgeFailureException error)
        {
            return await FailAsync(error.Code, error.Message, error.Details).ConfigureAwait(false);
        }
        catch (ProtonApiException error)
        {
            var code = error.TransportCode ?? 502;
            return await FailAsync(code, "proton api request failed", error.Message).ConfigureAwait(false);
        }
        catch (Exception error)
        {
            return await FailAsync(500, "bridge execution failed", error.ToString()).ConfigureAwait(false);
        }
    }

    private static async Task<int> HandleAuthAsync()
    {
        var request = await ReadRequestAsync<AuthRequest>().ConfigureAwait(false);

        await UseDriveClientAsync(
            request,
            static (_, _) => ValueTask.CompletedTask).ConfigureAwait(false);

        var username = RequireTrimmed(request.Username, "username");
        await WriteSuccessAsync(new
        {
            authenticated = true,
            username = username
        }).ConfigureAwait(false);

        return 0;
    }

    private static async Task<int> HandleUploadAsync()
    {
        var request = await ReadRequestAsync<UploadRequest>().ConfigureAwait(false);

        var response = await UseDriveClientAsync(request, async (client, cancellationToken) =>
        {
            var oid = NormalizeOid(request.Oid);
            var sourcePath = RequireFile(request.Path);
            var storageBase = NormalizeStorageBase(request.StorageBase);
            var storageSegments = SplitStorageBase(storageBase);

            var baseFolder = await ResolveFolderPathAsync(client, storageSegments, createMissing: true, cancellationToken).ConfigureAwait(false)
                             ?? throw new BridgeFailureException(500, $"failed to resolve storage root: {storageBase}");

            var prefix = oid.Substring(0, 2);
            var suffix = oid.Substring(2);

            var prefixFolder = await EnsureChildFolderAsync(client, baseFolder.Uid, prefix, cancellationToken).ConfigureAwait(false);
            var existing = await FindChildFileAsync(client, prefixFolder.Uid, suffix, cancellationToken).ConfigureAwait(false);

            if (existing is null)
            {
                var sourceInfo = new FileInfo(sourcePath);
                await using var uploader = await client.GetFileUploaderAsync(
                    prefixFolder.Uid,
                    suffix,
                    mediaType: "application/octet-stream",
                    size: sourceInfo.Length,
                    lastModificationTime: sourceInfo.LastWriteTimeUtc,
                    additionalMetadata: null,
                    overrideExistingDraftByOtherClient: false,
                    cancellationToken).ConfigureAwait(false);

                await using var uploadController = uploader.UploadFromFile(
                    sourcePath,
                    Array.Empty<Thumbnail>(),
                    null,
                    cancellationToken);

                await uploadController.Completion.ConfigureAwait(false);

                existing = await FindChildFileAsync(client, prefixFolder.Uid, suffix, cancellationToken).ConfigureAwait(false);
            }

            if (existing is null)
            {
                throw new BridgeFailureException(500, "upload completed but file metadata was not found");
            }

            var claimedSize = existing.ActiveRevision.ClaimedSize;
            var size = claimedSize is > 0 ? claimedSize.Value : new FileInfo(sourcePath).Length;

            return new
            {
                oid = oid,
                size = size,
                location = $"{storageBase}/{prefix}/{suffix}",
                nodeUid = existing.Uid.ToString(),
                revisionUid = existing.ActiveRevision.Uid.ToString()
            };
        }).ConfigureAwait(false);

        await WriteSuccessAsync(response).ConfigureAwait(false);
        return 0;
    }

    private static async Task<int> HandleDownloadAsync()
    {
        var request = await ReadRequestAsync<DownloadRequest>().ConfigureAwait(false);

        var response = await UseDriveClientAsync(request, async (client, cancellationToken) =>
        {
            var oid = NormalizeOid(request.Oid);
            var outputPath = RequireTrimmed(request.OutputPath, "outputPath");
            var storageBase = NormalizeStorageBase(request.StorageBase);
            var storageSegments = SplitStorageBase(storageBase);

            var baseFolder = await ResolveFolderPathAsync(client, storageSegments, createMissing: false, cancellationToken).ConfigureAwait(false);
            if (baseFolder is null)
            {
                throw new BridgeFailureException(404, $"storage root not found: {storageBase}");
            }

            var prefix = oid.Substring(0, 2);
            var suffix = oid.Substring(2);

            var prefixFolder = await FindChildFolderAsync(client, baseFolder.Uid, prefix, cancellationToken).ConfigureAwait(false);
            if (prefixFolder is null)
            {
                throw new BridgeFailureException(404, $"object not found for oid: {oid}");
            }

            var fileNode = await FindChildFileAsync(client, prefixFolder.Uid, suffix, cancellationToken).ConfigureAwait(false);
            if (fileNode is null)
            {
                throw new BridgeFailureException(404, $"object not found for oid: {oid}");
            }

            var outputDir = Path.GetDirectoryName(outputPath);
            if (!string.IsNullOrWhiteSpace(outputDir))
            {
                Directory.CreateDirectory(outputDir);
            }

            await using var downloader = await client.GetFileDownloaderAsync(fileNode.ActiveRevision.Uid, cancellationToken).ConfigureAwait(false);
            await using var downloadController = downloader.DownloadToFile(
                outputPath,
                static (_, _) => { },
                cancellationToken);

            await downloadController.Completion.ConfigureAwait(false);

            var computedOid = await ComputeFileSha256Async(outputPath, cancellationToken).ConfigureAwait(false);
            if (!string.Equals(computedOid, oid, StringComparison.Ordinal))
            {
                throw new BridgeFailureException(409, $"downloaded object hash mismatch for oid {oid}");
            }

            var info = new FileInfo(outputPath);
            return new
            {
                oid = oid,
                size = info.Length,
                path = outputPath,
                location = $"{storageBase}/{prefix}/{suffix}"
            };
        }).ConfigureAwait(false);

        await WriteSuccessAsync(response).ConfigureAwait(false);
        return 0;
    }

    private static async Task<int> HandleListAsync()
    {
        var request = await ReadRequestAsync<ListRequest>().ConfigureAwait(false);

        var response = await UseDriveClientAsync(request, async (client, cancellationToken) =>
        {
            var folder = NormalizeStorageBase(string.IsNullOrWhiteSpace(request.Folder) ? request.StorageBase : request.Folder);
            var folderSegments = SplitStorageBase(folder);

            var rootFolder = await ResolveFolderPathAsync(client, folderSegments, createMissing: false, cancellationToken).ConfigureAwait(false);
            if (rootFolder is null)
            {
                return new { files = Array.Empty<object>() };
            }

            var files = new List<object>();

            await foreach (var prefixResult in client.EnumerateFolderChildrenAsync(rootFolder.Uid, cancellationToken).ConfigureAwait(false))
            {
                if (!prefixResult.TryGetValueElseError(out var prefixNode, out _))
                {
                    continue;
                }

                if (prefixNode is not FolderNode prefixFolder || prefixFolder.Name.Length != 2)
                {
                    continue;
                }

                await foreach (var childResult in client.EnumerateFolderChildrenAsync(prefixFolder.Uid, cancellationToken).ConfigureAwait(false))
                {
                    if (!childResult.TryGetValueElseError(out var childNode, out _))
                    {
                        continue;
                    }

                    if (childNode is not FileNode fileNode)
                    {
                        continue;
                    }

                    var oid = (prefixFolder.Name + fileNode.Name).ToLowerInvariant();
                    if (!OidPattern.IsMatch(oid))
                    {
                        continue;
                    }

                    var modifiedTime = fileNode.ActiveRevision.ClaimedModificationTime ?? fileNode.ActiveRevision.CreationTime;
                    files.Add(new
                    {
                        oid = oid,
                        name = fileNode.Name,
                        size = fileNode.ActiveRevision.ClaimedSize ?? 0,
                        modified = modifiedTime.ToString("o"),
                        location = $"{folder}/{prefixFolder.Name}/{fileNode.Name}"
                    });
                }
            }

            return new { files = files };
        }).ConfigureAwait(false);

        await WriteSuccessAsync(response).ConfigureAwait(false);
        return 0;
    }

    private static async ValueTask<T> UseDriveClientAsync<T>(BridgeRequestBase request, Func<ProtonDriveClient, CancellationToken, ValueTask<T>> action)
    {
        var username = RequireTrimmed(request.Username, "username");
        var password = RequireValue(request.Password, "password");
        var appVersion = string.IsNullOrWhiteSpace(request.AppVersion) ? "external-drive-protonlfs@dev" : request.AppVersion.Trim();

        var timeoutSeconds = ParsePositiveIntEnvironment("PROTON_REAL_BRIDGE_TIMEOUT_SECONDS", 300);
        using var cancellationTokenSource = new CancellationTokenSource(TimeSpan.FromSeconds(timeoutSeconds));
        var cancellationToken = cancellationTokenSource.Token;

        var passwordBytes = Encoding.UTF8.GetBytes(password);
        var session = await ProtonApiSession.BeginAsync(username, passwordBytes, appVersion, cancellationToken).ConfigureAwait(false);

        try
        {
            if (session.IsWaitingForSecondFactorCode)
            {
                var secondFactorCode = RequireTrimmed(request.SecondFactorCode, "secondFactorCode");
                await session.ApplySecondFactorCodeAsync(secondFactorCode, cancellationToken).ConfigureAwait(false);
            }

            if (session.PasswordMode == PasswordMode.Dual)
            {
                var dataPassword = string.IsNullOrWhiteSpace(request.DataPassword)
                    ? password
                    : request.DataPassword;
                var dataPasswordBytes = Encoding.UTF8.GetBytes(dataPassword);
                await session.ApplyDataPasswordAsync(dataPasswordBytes, cancellationToken).ConfigureAwait(false);
            }

            var driveClient = new ProtonDriveClient(session);
            return await action(driveClient, cancellationToken).ConfigureAwait(false);
        }
        finally
        {
            try
            {
                await session.EndAsync().ConfigureAwait(false);
            }
            catch
            {
                // Ignore session termination failures at this layer.
            }
        }
    }

    private static async ValueTask<FolderNode?> ResolveFolderPathAsync(
        ProtonDriveClient client,
        IReadOnlyList<string> pathSegments,
        bool createMissing,
        CancellationToken cancellationToken)
    {
        var current = await client.GetMyFilesFolderAsync(cancellationToken).ConfigureAwait(false);
        foreach (var segment in pathSegments)
        {
            var folder = await FindChildFolderAsync(client, current.Uid, segment, cancellationToken).ConfigureAwait(false);
            if (folder is null && createMissing)
            {
                folder = await EnsureChildFolderAsync(client, current.Uid, segment, cancellationToken).ConfigureAwait(false);
            }

            if (folder is null)
            {
                return null;
            }

            current = folder;
        }
        return current;
    }

    private static async ValueTask<FolderNode> EnsureChildFolderAsync(
        ProtonDriveClient client,
        NodeUid parentUid,
        string folderName,
        CancellationToken cancellationToken)
    {
        var existing = await FindChildFolderAsync(client, parentUid, folderName, cancellationToken).ConfigureAwait(false);
        if (existing is not null)
        {
            return existing;
        }

        try
        {
            return await client.CreateFolderAsync(parentUid, folderName, DateTime.UtcNow, cancellationToken).ConfigureAwait(false);
        }
        catch (NodeWithSameNameExistsException)
        {
            var refreshed = await FindChildFolderAsync(client, parentUid, folderName, cancellationToken).ConfigureAwait(false);
            if (refreshed is null)
            {
                throw new BridgeFailureException(500, $"failed to resolve folder after create race: {folderName}");
            }
            return refreshed;
        }
    }

    private static async ValueTask<FolderNode?> FindChildFolderAsync(
        ProtonDriveClient client,
        NodeUid parentUid,
        string folderName,
        CancellationToken cancellationToken)
    {
        await foreach (var result in client.EnumerateFolderChildrenAsync(parentUid, cancellationToken).ConfigureAwait(false))
        {
            if (!result.TryGetValueElseError(out var node, out _))
            {
                continue;
            }
            if (node is FolderNode folder && string.Equals(folder.Name, folderName, StringComparison.Ordinal))
            {
                return folder;
            }
        }
        return null;
    }

    private static async ValueTask<FileNode?> FindChildFileAsync(
        ProtonDriveClient client,
        NodeUid parentUid,
        string fileName,
        CancellationToken cancellationToken)
    {
        await foreach (var result in client.EnumerateFolderChildrenAsync(parentUid, cancellationToken).ConfigureAwait(false))
        {
            if (!result.TryGetValueElseError(out var node, out _))
            {
                continue;
            }
            if (node is FileNode file && string.Equals(file.Name, fileName, StringComparison.Ordinal))
            {
                return file;
            }
        }
        return null;
    }

    private static async Task<string> ComputeFileSha256Async(string path, CancellationToken cancellationToken)
    {
        await using var stream = File.OpenRead(path);
        var hash = await SHA256.HashDataAsync(stream, cancellationToken).ConfigureAwait(false);
        return Convert.ToHexString(hash).ToLowerInvariant();
    }

    private static string NormalizeOid(string? oid)
    {
        var normalized = RequireTrimmed(oid, "oid").ToLowerInvariant();
        if (!OidPattern.IsMatch(normalized))
        {
            throw new BridgeFailureException(400, $"invalid oid format: {normalized}");
        }
        return normalized;
    }

    private static string RequireFile(string? path)
    {
        var normalized = RequireTrimmed(path, "path");
        if (!File.Exists(normalized))
        {
            throw new BridgeFailureException(404, $"file not found: {normalized}");
        }
        return normalized;
    }

    private static string RequireValue(string? value, string name)
    {
        if (value is null)
        {
            throw new BridgeFailureException(400, $"missing required field: {name}");
        }
        return value;
    }

    private static string RequireTrimmed(string? value, string name)
    {
        var normalized = (value ?? string.Empty).Trim();
        if (string.IsNullOrWhiteSpace(normalized))
        {
            throw new BridgeFailureException(400, $"missing required field: {name}");
        }
        return normalized;
    }

    private static string NormalizeStorageBase(string? storageBase)
    {
        var value = string.IsNullOrWhiteSpace(storageBase) ? "LFS" : storageBase.Trim();
        value = value.Trim('/');
        if (string.IsNullOrWhiteSpace(value))
        {
            throw new BridgeFailureException(400, "storage base cannot be empty");
        }
        return value;
    }

    private static IReadOnlyList<string> SplitStorageBase(string storageBase)
    {
        var segments = storageBase
            .Split(new[] { '/', '\\' }, StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries)
            .Where(segment => !string.IsNullOrWhiteSpace(segment))
            .ToArray();

        if (segments.Length == 0)
        {
            throw new BridgeFailureException(400, "storage base must contain at least one path segment");
        }
        return segments;
    }

    private static int ParsePositiveIntEnvironment(string name, int fallback)
    {
        var raw = Environment.GetEnvironmentVariable(name);
        if (string.IsNullOrWhiteSpace(raw))
        {
            return fallback;
        }

        return int.TryParse(raw.Trim(), out var value) && value > 0
            ? value
            : fallback;
    }

    private static async Task<T> ReadRequestAsync<T>()
        where T : class, new()
    {
        var raw = await Console.In.ReadToEndAsync().ConfigureAwait(false);
        if (string.IsNullOrWhiteSpace(raw))
        {
            return new T();
        }

        var request = JsonSerializer.Deserialize<T>(raw, JsonOptions);
        if (request is null)
        {
            throw new BridgeFailureException(400, "invalid request payload");
        }
        return request;
    }

    private static Task WriteSuccessAsync(object payload)
    {
        return WriteJsonAsync(new
        {
            ok = true,
            payload
        });
    }

    private static async Task<int> FailAsync(int code, string error, string details = "")
    {
        await WriteJsonAsync(new
        {
            ok = false,
            code,
            error,
            details
        }).ConfigureAwait(false);
        return 1;
    }

    private static Task WriteJsonAsync(object payload)
    {
        var serialized = JsonSerializer.Serialize(payload, JsonOptions);
        return Console.Out.WriteLineAsync(serialized);
    }

    private sealed class BridgeFailureException : Exception
    {
        public BridgeFailureException(int code, string message, string details = "")
            : base(message)
        {
            Code = code;
            Details = details;
        }

        public int Code { get; }
        public string Details { get; }
    }

    private class BridgeRequestBase
    {
        public string? Username { get; set; }
        public string? Password { get; set; }
        public string? DataPassword { get; set; }
        public string? SecondFactorCode { get; set; }
        public string? AppVersion { get; set; }
    }

    private sealed class AuthRequest : BridgeRequestBase
    {
    }

    private sealed class UploadRequest : BridgeRequestBase
    {
        public string? Oid { get; set; }
        public string? Path { get; set; }
        public string? StorageBase { get; set; }
    }

    private sealed class DownloadRequest : BridgeRequestBase
    {
        public string? Oid { get; set; }
        public string? OutputPath { get; set; }
        public string? StorageBase { get; set; }
    }

    private sealed class ListRequest : BridgeRequestBase
    {
        public string? Folder { get; set; }
        public string? StorageBase { get; set; }
    }
}
