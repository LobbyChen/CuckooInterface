using System;
using System.Collections.Generic;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Input;
using System.Windows.Media;

namespace CuckooInterfaceUI.Behaviors
{
    /// <summary>
    /// 为 ScrollViewer 提供平滑滚动行为。
    /// 通过附加属性启用后，鼠标滚轮不再直接跳转到目标位置，
    /// 而是在每帧向目标偏移量插值，实现平滑过渡。
    /// </summary>
    public static class SmoothScrollBehavior
    {
        // ===== 附加属性：是否启用平滑滚动 =====

        public static readonly DependencyProperty IsEnabledProperty =
            DependencyProperty.RegisterAttached(
                "IsEnabled",
                typeof(bool),
                typeof(SmoothScrollBehavior),
                new PropertyMetadata(false, OnIsEnabledChanged));

        public static void SetIsEnabled(DependencyObject element, bool value)
            => element.SetValue(IsEnabledProperty, value);

        public static bool GetIsEnabled(DependencyObject element)
            => (bool)element.GetValue(IsEnabledProperty);

        // ===== 内部状态 =====

        /// <summary>记录每个 ScrollViewer 的平滑滚动状态。</summary>
        private static readonly Dictionary<ScrollViewer, ScrollState> _states = new();

        private sealed class ScrollState
        {
            public double TargetOffset;   // 目标垂直偏移
            public bool IsAnimating;     // 是否正在动画中
        }

        private static void OnIsEnabledChanged(DependencyObject d, DependencyPropertyChangedEventArgs e)
        {
            if (d is not ScrollViewer sv) return;

            if ((bool)e.NewValue)
            {
                sv.PreviewMouseWheel += OnPreviewMouseWheel;
                sv.Unloaded += OnScrollViewerUnloaded;
            }
            else
            {
                sv.PreviewMouseWheel -= OnPreviewMouseWheel;
                sv.Unloaded -= OnScrollViewerUnloaded;
                _states.Remove(sv);
            }
        }

        private static void OnScrollViewerUnloaded(object sender, RoutedEventArgs e)
        {
            if (sender is ScrollViewer sv)
            {
                sv.PreviewMouseWheel -= OnPreviewMouseWheel;
                sv.Unloaded -= OnScrollViewerUnloaded;
                _states.Remove(sv);
            }
        }

        private static void OnPreviewMouseWheel(object sender, MouseWheelEventArgs e)
        {
            if (sender is not ScrollViewer sv) return;

            // 若鼠标在另一个可滚动的子 ScrollViewer 内，则让子控件自行处理
            if (e.OriginalSource is DependencyObject src)
            {
                var child = FindParent<ScrollViewer>(src);
                if (child != null && child != sv && child.ExtentHeight > child.ViewportHeight)
                {
                    return;
                }
            }

            if (!_states.TryGetValue(sv, out var state))
            {
                state = new ScrollState { TargetOffset = sv.VerticalOffset };
                _states[sv] = state;
            }

            // 计算目标偏移量（WPF 滚轮 e.Delta 通常为 120 的倍数）
            const double scrollFactor = 1.0; // 可调整灵敏度
            double delta = e.Delta * scrollFactor;
            state.TargetOffset = Math.Clamp(
                state.TargetOffset - delta,
                0,
                sv.ScrollableHeight);

            if (!state.IsAnimating)
            {
                state.IsAnimating = true;
                CompositionTarget.Rendering += OnRendering;
            }

            e.Handled = true;
        }

        private static void OnRendering(object? sender, EventArgs e)
        {
            if (_states.Count == 0)
            {
                CompositionTarget.Rendering -= OnRendering;
                return;
            }

            List<ScrollViewer>? toRemove = null;
            foreach (var kv in _states)
            {
                var sv = kv.Key;
                var state = kv.Value;

                double current = sv.VerticalOffset;
                double diff = state.TargetOffset - current;

                // 接近目标时直接吸附并停止
                if (Math.Abs(diff) < 0.5)
                {
                    sv.ScrollToVerticalOffset(state.TargetOffset);
                    state.IsAnimating = false;
                    toRemove ??= new List<ScrollViewer>();
                    toRemove.Add(sv);
                    continue;
                }

                // 指数插值：每帧向目标移动约 12%，越接近越慢，产生缓出效果
                double step = diff * 0.12;
                sv.ScrollToVerticalOffset(current + step);
            }

            if (toRemove != null)
            {
                foreach (var sv in toRemove)
                {
                    _states.Remove(sv);
                }
            }

            if (_states.Count == 0)
            {
                CompositionTarget.Rendering -= OnRendering;
            }
        }

        private static T? FindParent<T>(DependencyObject child) where T : DependencyObject
        {
            var parent = VisualTreeHelper.GetParent(child);
            while (parent != null)
            {
                if (parent is T typed) return typed;
                parent = VisualTreeHelper.GetParent(parent);
            }
            return null;
        }
    }
}
