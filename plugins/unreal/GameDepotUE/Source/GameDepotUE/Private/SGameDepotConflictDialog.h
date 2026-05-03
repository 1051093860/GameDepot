#pragma once

#include "CoreMinimal.h"
#include "Widgets/SCompoundWidget.h"
#include "Widgets/Views/SListView.h"

struct FGameDepotConflictItem
{
    FString Path;
    FString Kind;
    FString BaseOID;
    FString LocalOID;
    FString RemoteOID;
};

using FGameDepotConflictItemPtr = TSharedPtr<FGameDepotConflictItem>;

DECLARE_DELEGATE_TwoParams(FGameDepotResolveConflictAction, const FString& /*Path*/, const FString& /*Decision*/);

class SGameDepotConflictDialog : public SCompoundWidget
{
public:
    SLATE_BEGIN_ARGS(SGameDepotConflictDialog) {}
        SLATE_ARGUMENT(TArray<FGameDepotConflictItemPtr>, Conflicts)
        SLATE_EVENT(FGameDepotResolveConflictAction, OnResolveRequested)
    SLATE_END_ARGS()

    void Construct(const FArguments& InArgs);

private:
    TArray<FGameDepotConflictItemPtr> Conflicts;
    FGameDepotResolveConflictAction OnResolveRequested;
    TSharedPtr<SListView<FGameDepotConflictItemPtr>> ListView;

    TSharedRef<ITableRow> MakeRow(FGameDepotConflictItemPtr Item, const TSharedRef<STableViewBase>& OwnerTable);
    FText ExplainKind(const FString& Kind) const;
};
