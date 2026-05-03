#pragma once

#include "CoreMinimal.h"

enum class EGameDepotStorage : uint8
{
    Git,
    OSS,
    NewToGit,
    NewToOSS,
    Review,
    Ignored
};

enum class EGameDepotSyncState : uint8
{
    Synced,
    Modified,
    MissingLocal,
    MissingRemote,
    RoutingConflict,
    NewFile,
    ReviewRequired,
    Ignored
};

enum class EGameDepotSeverity : uint8
{
    Good,
    Info,
    Warning,
    Error,
    Muted
};

struct FGameDepotHistoryEntry
{
    FString CommitId;
    FString Message;
    FString ShortHash;
    EGameDepotStorage Storage = EGameDepotStorage::Git;
    int64 SizeBytes = 0;
    FDateTime CommitDate;
};

struct FGameDepotAssetRow
{
    FString DepotPath;
    FString PackageName;
    FString AssetName;
    FString ClassName;
    EGameDepotStorage Storage = EGameDepotStorage::Review;
    EGameDepotSyncState Sync = EGameDepotSyncState::ReviewRequired;
    EGameDepotSeverity Severity = EGameDepotSeverity::Warning;
    FString DesiredRule;
    FString ChangeState;
    FString Kind;
    FString RemoteState;
    FString Message;
    FString ShortHash;
    bool bGitTracked = false;
    bool bLocalExists = true;
    bool bRemoteExists = false;
    bool bRemoteChecked = true;
    bool bLocalBlobCached = false;
    bool bHistoryOnly = false;
    bool bSelected = false;
};

using FGameDepotAssetRowPtr = TSharedPtr<FGameDepotAssetRow>;

namespace GameDepotStatusText
{
    FString ToStorageText(EGameDepotStorage Storage);
    FString ToSyncText(EGameDepotSyncState Sync);
    FString ToSeverityText(EGameDepotSeverity Severity);
    FSlateColor ToSeverityColor(EGameDepotSeverity Severity);
    FSlateColor ToStorageColor(EGameDepotStorage Storage);
    FSlateColor ToSyncColor(EGameDepotSyncState Sync);
}
