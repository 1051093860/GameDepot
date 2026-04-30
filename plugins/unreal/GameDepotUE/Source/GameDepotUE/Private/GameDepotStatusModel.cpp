#include "GameDepotStatusModel.h"

namespace GameDepotStatusText
{
    FString ToStorageText(EGameDepotStorage Storage)
    {
        switch (Storage)
        {
        case EGameDepotStorage::Git: return TEXT("Git");
        case EGameDepotStorage::OSS: return TEXT("OSS");
        case EGameDepotStorage::NewToGit: return TEXT("New -> Git");
        case EGameDepotStorage::NewToOSS: return TEXT("New -> OSS");
        case EGameDepotStorage::Ignored: return TEXT("Ignored");
        default: return TEXT("Review");
        }
    }

    FString ToSyncText(EGameDepotSyncState Sync)
    {
        switch (Sync)
        {
        case EGameDepotSyncState::Synced: return TEXT("Synced");
        case EGameDepotSyncState::Modified: return TEXT("Modified");
        case EGameDepotSyncState::MissingLocal: return TEXT("Missing Local");
        case EGameDepotSyncState::MissingRemote: return TEXT("Missing OSS");
        case EGameDepotSyncState::RoutingConflict: return TEXT("Routing Conflict");
        case EGameDepotSyncState::NewFile: return TEXT("New");
        case EGameDepotSyncState::Ignored: return TEXT("Ignored");
        default: return TEXT("Needs Rule");
        }
    }

    FString ToSeverityText(EGameDepotSeverity Severity)
    {
        switch (Severity)
        {
        case EGameDepotSeverity::Good: return TEXT("Good");
        case EGameDepotSeverity::Info: return TEXT("Info");
        case EGameDepotSeverity::Warning: return TEXT("Warning");
        case EGameDepotSeverity::Error: return TEXT("Error");
        default: return TEXT("Muted");
        }
    }

    FSlateColor ToSeverityColor(EGameDepotSeverity Severity)
    {
        switch (Severity)
        {
        case EGameDepotSeverity::Good: return FSlateColor(FLinearColor(0.20f, 0.78f, 0.36f));
        case EGameDepotSeverity::Info: return FSlateColor(FLinearColor(0.35f, 0.62f, 1.00f));
        case EGameDepotSeverity::Warning: return FSlateColor(FLinearColor(1.00f, 0.74f, 0.25f));
        case EGameDepotSeverity::Error: return FSlateColor(FLinearColor(1.00f, 0.28f, 0.22f));
        default: return FSlateColor(FLinearColor(0.55f, 0.55f, 0.55f));
        }
    }

    FSlateColor ToStorageColor(EGameDepotStorage Storage)
    {
        switch (Storage)
        {
        case EGameDepotStorage::Git: return FSlateColor(FLinearColor(0.42f, 0.75f, 1.00f));
        case EGameDepotStorage::OSS: return FSlateColor(FLinearColor(0.86f, 0.56f, 1.00f));
        case EGameDepotStorage::NewToGit: return FSlateColor(FLinearColor(0.50f, 0.86f, 1.00f));
        case EGameDepotStorage::NewToOSS: return FSlateColor(FLinearColor(0.96f, 0.70f, 1.00f));
        case EGameDepotStorage::Ignored: return FSlateColor(FLinearColor(0.55f, 0.55f, 0.55f));
        default: return FSlateColor(FLinearColor(1.00f, 0.74f, 0.25f));
        }
    }

    FSlateColor ToSyncColor(EGameDepotSyncState Sync)
    {
        switch (Sync)
        {
        case EGameDepotSyncState::Synced: return FSlateColor(FLinearColor(0.20f, 0.78f, 0.36f));
        case EGameDepotSyncState::Modified: return FSlateColor(FLinearColor(1.00f, 0.74f, 0.25f));
        case EGameDepotSyncState::MissingLocal:
        case EGameDepotSyncState::MissingRemote:
        case EGameDepotSyncState::RoutingConflict: return FSlateColor(FLinearColor(1.00f, 0.28f, 0.22f));
        case EGameDepotSyncState::NewFile: return FSlateColor(FLinearColor(0.35f, 0.62f, 1.00f));
        case EGameDepotSyncState::Ignored: return FSlateColor(FLinearColor(0.55f, 0.55f, 0.55f));
        default: return FSlateColor(FLinearColor(1.00f, 0.74f, 0.25f));
        }
    }
}
