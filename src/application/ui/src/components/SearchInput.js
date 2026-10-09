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

import React, { useState, useEffect, useRef, memo } from 'react';
import {
    TextField,
    InputAdornment,
    Button
} from '@mui/material';
import SearchIcon from '@mui/icons-material/Search';

// SearchInput component that isolates input handling from parent component
const SearchInput = memo(({ onSearch }) => {
    const [inputValue, setInputValue] = useState('');
    const debounceTimerRef = useRef(null);

    // Handle input change without propagating all changes to parent
    const handleInputChange = (e) => {
        setInputValue(e.target.value);
    };

    // Submit search only when user explicitly requests it
    const handleSubmitSearch = () => {
        onSearch(inputValue);
    };

    // Handle Enter key press
    const handleKeyDown = (event) => {
        if (event.key === 'Enter') {
            handleSubmitSearch();
        }
    };

    // Clean up timer on unmount
    useEffect(() => {
        return () => {
            if (debounceTimerRef.current) {
                clearTimeout(debounceTimerRef.current);
            }
        };
    }, []);

    return (
        <TextField
            fullWidth
            label="Search products"
            variant="outlined"
            value={inputValue}
            onChange={handleInputChange}
            onKeyDown={handleKeyDown}
            InputProps={{
                endAdornment: (
                    <InputAdornment position="end">
                        <Button
                            variant="contained"
                            color="primary"
                            onClick={handleSubmitSearch}
                            startIcon={<SearchIcon />}
                        >
                            Search
                        </Button>
                    </InputAdornment>
                )
            }}
            sx={{ mb: 3 }}
        />
    );
});

export default SearchInput;
