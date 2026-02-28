"""Tests for resource detection."""


def test_resource_info_fields():
    """ResourceInfo should have all required fields."""
    from worker.resources.detector import ResourceInfo

    info = ResourceInfo(
        gpu_name="Test GPU",
        vram_total_mb=8192,
        vram_free_mb=6144,
        ram_total_mb=32768,
        ram_free_mb=24576,
        cuda_available=True,
        platform="Windows",
    )

    assert info.gpu_name == "Test GPU"
    assert info.vram_total_mb == 8192
    assert info.vram_free_mb == 6144
    assert info.ram_total_mb == 32768
    assert info.ram_free_mb == 24576
    assert info.cuda_available is True
    assert info.platform == "Windows"
