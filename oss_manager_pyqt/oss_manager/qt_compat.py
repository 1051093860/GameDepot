try:
    from PyQt6 import QtCore, QtGui, QtWidgets
    Signal = QtCore.pyqtSignal
    Slot = QtCore.pyqtSlot
    QT_API = "PyQt6"
except Exception:  # pragma: no cover
    from PySide6 import QtCore, QtGui, QtWidgets  # type: ignore
    Signal = QtCore.Signal
    Slot = QtCore.Slot
    QT_API = "PySide6"
