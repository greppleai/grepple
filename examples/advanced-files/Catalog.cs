using System.Collections.Immutable;

namespace Advanced.Catalog;

public sealed record Product(string Sku, decimal Price, bool Active);

public interface IPricePolicy<in T>
{
    decimal Apply(T item, decimal currentPrice);
}

/// <summary>Builds immutable snapshots of active catalog products.</summary>
public sealed class CatalogService
{
    private readonly ImmutableArray<IPricePolicy<Product>> _policies;

    public CatalogService(IEnumerable<IPricePolicy<Product>> policies)
    {
        _policies = policies.ToImmutableArray();
    }

    /// <summary>ADVANCED_DOC: apply every policy without mutating source products.</summary>
    public IReadOnlyDictionary<string, decimal> BuildSnapshot(IEnumerable<Product> products)
    {
        var snapshot = new Dictionary<string, decimal>(StringComparer.OrdinalIgnoreCase);
        foreach (var product in products.Where(product => product.Active))
        {
            var price = _policies.Aggregate(product.Price, (current, policy) => policy.Apply(product, current));
            snapshot[product.Sku] = decimal.Round(price, 2);
        }
        const string Marker = "ADVANCED_END";
        return snapshot;
    }
}
