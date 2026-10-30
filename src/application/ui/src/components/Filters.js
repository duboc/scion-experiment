/*
 * Copyright 2025 Google LLC
 * 
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 * 
 *     https://www.apache.org/licenses/LICENSE-2.0
 * 
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import React, { useState } from 'react';
import './Filters.css';

// Helper component for each filter section
const FilterSection = ({ filterConfig, selectedOptions, onOptionChange }) => {
    const [isOpen, setIsOpen] = useState(true);
    const [showAll, setShowAll] = useState(false);

    // Number of options to show initially
    const initialOptionCount = 5;

    // Determine if we need a "See more" button
    const needsShowMore = filterConfig.options.length > initialOptionCount;

    // Ensure any selected options beyond initialOptionCount remain visible
    const baseOptions = showAll ? filterConfig.options : filterConfig.options.slice(0, initialOptionCount);
    const hiddenSelected = showAll
        ? []
        : filterConfig.options.slice(initialOptionCount).filter((opt) => {
            const val = opt.value !== undefined ? opt.value : opt;
            return Boolean(opt.selected) || selectedOptions.some((s) => String(s).toLowerCase() === String(val).toLowerCase());
        });
    const displayOptions = [...baseOptions, ...hiddenSelected];

    const toggleOpen = () => setIsOpen(!isOpen);
    const toggleShowAll = () => setShowAll(!showAll);

    return (
        <div className="filter-section">
            <div className="filter-header" onClick={toggleOpen}>
                <h3>
                    {filterConfig.title}
                    {selectedOptions.length > 0 && (
                        <span style={{ marginLeft: '6px', fontSize: '11px', color: '#2563eb', fontWeight: 600 }}>
                            ({selectedOptions.length})
                        </span>
                    )}
                </h3>
                <span className={`arrow ${isOpen ? 'up' : 'down'}`}></span>
            </div>
            {isOpen && (
                <div className="filter-options">
                    {displayOptions.map((option) => {
                        const optVal = option.value !== undefined ? option.value : option;
                        const isChecked = Boolean(option.selected) || selectedOptions.some(
                            (s) => String(s).toLowerCase() === String(optVal).toLowerCase()
                        );
                        return (
                            <div key={optVal} className="filter-option">
                                <input
                                    type="checkbox"
                                    id={`${filterConfig.id}-${optVal}`}
                                    value={optVal}
                                    checked={isChecked}
                                    onChange={(e) => onOptionChange(e, filterConfig.id)}
                                />
                                <label htmlFor={`${filterConfig.id}-${optVal}`}>
                                    {option.label || option}
                                    {option.count !== undefined && !String(option.label || '').includes(`(${option.count})`) && (
                                        <span className="filter-count"> ({option.count})</span>
                                    )}
                                </label>
                            </div>
                        );
                    })}

                    {/* Show "See more" button if needed */}
                    {needsShowMore && (
                        <button
                            className="see-more"
                            onClick={toggleShowAll}
                        >
                            {showAll ? "See less" : "See more"}
                        </button>
                    )}
                </div>
            )}
        </div>
    );
};

/**
 * Dynamic Filters component that renders filter sections based on configuration
 * @param {Array} filterConfigs - Array of filter configuration objects
 * @param {Object} selectedFilters - Object with selected filter values by filter ID
 * @param {Function} onFilterChange - Callback for filter changes
 * @param {Function} onClearAll - Callback to clear all active filters
 * @param {number|null} facetsLatencyMs - Latency in ms for server-side facets computation
 */
const Filters = ({ filterConfigs = [], selectedFilters = {}, onFilterChange, onClearAll, facetsLatencyMs = null }) => {
    // Handler that processes the checkbox changes and calls the parent component's handler
    const handleCheckboxChange = (event, filterId) => {
        const { value, checked } = event.target;

        // Call the actual handler passed from App.js
        console.log(`Filter change: ${filterId} - ${value} - ${checked}`);
        onFilterChange(filterId, value, checked);
    };

    const hasActiveSelections = Object.values(selectedFilters || {}).some(
        (arr) => Array.isArray(arr) && arr.length > 0
    );

    // If no filter configs provided, show a message
    if (!filterConfigs.length) {
        return <div className="filters-container">No filters available</div>;
    }

    return (
        <div className="filters-container">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '8px' }}>
                <h2>Filters</h2>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    {facetsLatencyMs !== null && facetsLatencyMs !== undefined && (
                        <span style={{ fontSize: '11px', color: '#64748b' }}>{Number(facetsLatencyMs).toFixed(1)}ms</span>
                    )}
                    {hasActiveSelections && onClearAll && (
                        <button
                            type="button"
                            onClick={onClearAll}
                            style={{
                                background: 'transparent',
                                border: 'none',
                                color: '#2563eb',
                                fontSize: '12px',
                                fontWeight: 600,
                                cursor: 'pointer',
                                padding: 0,
                                textDecoration: 'underline',
                            }}
                        >
                            Clear all
                        </button>
                    )}
                </div>
            </div>

            {filterConfigs.map(filterConfig => (
                <FilterSection
                    key={filterConfig.id}
                    filterConfig={filterConfig}
                    selectedOptions={selectedFilters[filterConfig.id] || []}
                    onOptionChange={handleCheckboxChange}
                />
            ))}
        </div>
    );
};

export default Filters;
