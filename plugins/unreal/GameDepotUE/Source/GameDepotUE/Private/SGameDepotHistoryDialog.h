#pragma once

#include "CoreMinimal.h"
#include "GameDepotStatusModel.h"
#include "Widgets/SCompoundWidget.h"

class FGameDepotMockStatusProvider;
class SWindow;
template<typename ItemType> class SListView;

typedef TSharedPtr<FGameDepotHistoryEntry> FGameDepotHistoryEntryPtr;
DECLARE_DELEGATE_TwoParams(FGameDepotRestoreVersionAction, const FString& /*DepotPath*/, const FGameDepotHistoryEntry& /*Entry*/);

class SGameDepotHistoryDialog : public SCompoundWidget
{
public:
    SLATE_BEGIN_ARGS(SGameDepotHistoryDialog) {}
        SLATE_ARGUMENT(FString, DepotPath)
        SLATE_ARGUMENT(TSharedPtr<FGameDepotMockStatusProvider>, Provider)
        SLATE_ARGUMENT(TWeakPtr<SWindow>, ParentWindow)
        SLATE_EVENT(FSimpleDelegate, OnRestored)
        SLATE_EVENT(FGameDepotRestoreVersionAction, OnRestoreVersion)
    SLATE_END_ARGS()

    void Construct(const FArguments& InArgs);

private:
    FString DepotPath;
    TSharedPtr<FGameDepotMockStatusProvider> Provider;
    TWeakPtr<SWindow> ParentWindow;
    FSimpleDelegate OnRestored;
    FGameDepotRestoreVersionAction OnRestoreVersion;

    TArray<FGameDepotHistoryEntryPtr> HistoryRows;
    TSharedPtr<SListView<FGameDepotHistoryEntryPtr>> HistoryListView;
    FGameDepotHistoryEntryPtr SelectedEntry;

    TSharedRef<ITableRow> MakeHistoryRow(FGameDepotHistoryEntryPtr Item, const TSharedRef<STableViewBase>& OwnerTable);
    void OnSelectionChanged(FGameDepotHistoryEntryPtr Item, ESelectInfo::Type SelectInfo);
    FReply OnRestoreClicked();
    FReply OnCancelClicked();

    FText GetSelectedSummary() const;
    static FText StorageText(EGameDepotStorage Storage);
    static FText SizeText(int64 SizeBytes);
};
