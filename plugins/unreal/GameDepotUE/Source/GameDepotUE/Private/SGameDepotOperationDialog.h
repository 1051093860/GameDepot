#pragma once

#include "CoreMinimal.h"
#include "Widgets/SCompoundWidget.h"

class SProgressBar;
class STextBlock;

class SGameDepotOperationDialog : public SCompoundWidget
{
public:
    SLATE_BEGIN_ARGS(SGameDepotOperationDialog) {}
        SLATE_ARGUMENT(FString, OperationName)
    SLATE_END_ARGS()

    void Construct(const FArguments& InArgs);

    void SetProgress(const FString& Phase, const FString& Message, int32 Current, int32 Total, int32 Percent);
    void SetSucceeded(const FString& Message);
    void SetFailed(const FString& Message);
    void AppendLog(const FString& Line);
    void SetLogs(const TArray<FString>& Lines);

private:
    FString OperationName;
    FString LogText;
    TSharedPtr<STextBlock> TitleText;
    TSharedPtr<STextBlock> PhaseText;
    TSharedPtr<STextBlock> CurrentText;
    TSharedPtr<STextBlock> LogTextBlock;
    TSharedPtr<SProgressBar> ProgressBar;
};
